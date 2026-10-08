# HTTP interno uniforme, sin gRPC

## Contexto

El blueprint original de este servicio proponía gRPC con Protobuf para hablar con el Extractor, con streaming de
bytes en RAM hacia él, un directorio `proto/` y un adaptador `grpc_extractor/`. Ese diseño tiene dos supuestos
que hoy no se cumplen.

El primero es que el Extractor consume un **stream de bytes**. El sketch del blueprint asumía que el documento
viaja en trozos, sin materializarse entero en ninguna de las dos puntas. El segundo es que existe un
contr compilable. Hoy no hay ningún repositorio de Extractor ni de Resumidor: los dos servicios tienen que ser
escritos, y el contrato entre ellos es un documento —`docs/contracts/workers.md`— que alguien tiene que leer y
discutir, no un `.proto` que un plugin genera.

Hay una presión adicional que conviene nombrar. Cuando el otro lado del contrato todavía no existe, cada capa
de abstracción que se pone entre las partes es una capa que no se ha ganado: hay que elegir serialización,
manejar timeouts, decidir sobre versiones, y mantener stubs de imagen para cada campo del mensaje. Nada de eso
se paga solo, y nada de eso se paga una vez. Lo único que sí se paga, para siempre, es el costo de revertir.

## Decisión

**Los tres workers se comunican con el Orquestador por HTTP/1.1 con cuerpos JSON.** Sin gRPC, sin Protobuf, sin
directorio `proto/`, sin generación de código, sin streaming de bytes. El mismo estilo en los tres, sin
excepciones: un solo camino mental para leer los logs de los cuatro procesos.

Esto se aplica a la ruta del Extractor (que es la que el blueprint cambiaba), a la del Resumidor, y a la del
Persistidor.

### Consecuencia directa: no hay streaming entre servicios

Esta es la parte que hay que decir explícitamente, porque el blueprint original estaba construido sobre ella y
cualquiera que lo lea después va a buscarla:

**El Extractor recibe el documento completo como cuerpo multipart, no como un stream de trozos.** El documento
existe entero en RAM en el Orquestador, viaja entero, y existe entero en RAM en el Extractor. No hay
`Transfer-Encoding: chunked` entre servicios, ni lectura incremental, ni pico de memoria acotado por el ancho
de banda.

Lo único que acota el tamaño es la puerta 1 del borde: si el documento supera el máximo configurable, no entra.
Ese techo es ahora la garantía de memoria, y por eso es un parámetro de configuración y no un detalle de
implementación.

El texto extraído, en cambio, puede ser mucho más grande que el documento del que salió, y viaja entero en un
campo JSON. No hay paginación ni compresión en esa ruta.

## Alternativas consideradas

**gRPC con Protobuf, como proponía el blueprint original.** Rechazada. Tres costos que no se pagan una sola vez:
stubs de imagen que hay que regenerar con cada cambio de contrato y que hay que mantener alineados entre
servicios; un modo de falla adicional (HTTP/2 y el framing de protobuf) encima del que ya existe; y una
dirección de reversión cara, porque revertir fuera de gRPC significa tirar el `.proto` y reescribir los
adaptadores. Es exactamente la opción que más esfuerzo cuesta deshacer, y se está eligiendo con el otro repo
sin existir.

**HTTP con el cuerpo en streaming.** Rechazada. El streaming sobre HTTP resuelve un problema que la puerta 1
ya resolvió: acotar la memoria. Con un techo de tamaño en el borde, el streaming agrega complejidad de manejo
de chunks, de cancelación y de errores parciales sin comprar ninguna garantía adicional.

**JSON con nombres de campo en `snake_case`.** Adoptada sin discusión como convención de todo el contrato
público. Se decide para que el JSON del stream, el de la API y el de los workers hablen el mismo idioma.

## Consecuencias

### Positivas

- Una sola tecnología en los cuatro procesos. Un `curl` contra el Resumidor con un log de acceso al lado
  diagnostica el 90% de los problemas internos.
- Los contratos son documentos que una persona revisa y discute. Es la única opción viable cuando el otro repo
  todavía no existe.
- Sin paso de generación de código: no hay `protoc`, ni `buf`, ni Makefile que se pueda desincronizar.
- Revertir el día que haga falta es barato: cambiar el transporte no toca los casos de uso ni el dominio,
  porque los ports ya están definidos por el consumidor.
- El tiempo de desarrollo de los workers no depende de que haya una versión de contratos compatible con la
  nuestra.

### Negativas / costo

- **No hay verificación de tipos en tiempo de compilación.** Si el Extractor renombra `extracted_text` y nadie
  actualiza `docs/contracts/workers.md`, el sistema no falla al compilar: falla en producción, con un `nil`
  silencioso o con un `422` que nadie sabe interpretar. Este es el costo real de esta decisión y el que más
  duele a escala.
- **La mitigación son los tests de contrato**, no el compilador. Los tests de `http_extractor` y
  `http_summarizer` tienen que validar el contrato contra un servidor real, y hay que aceptar que un cambio de
  contrato que no actualiza el test falla de una forma confusa.
- **Pico de memoria acotado solo por la puerta 1**, no por el transporte. El documento está en RAM en las dos
  puntas y el texto extraído entero en una respuesta JSON.
- **Los timeouts son manuales.** No hay deadlines de gRPC: cada cliente define su propio, y equivocarse en eso
  no lo detecta ningún compilador. El caso más delicado es el cliente del Resumidor, cuyo timeout tiene que ser
  holgado por diseño (el modelo local está serializado) y por eso tiene que ir acompañado del semáforo angosto
  que describe ADR-0008.
- **Un JSON con el texto completo es verboso.** Irrelevante frente a la latencia de un modelo de lenguaje, pero
  es un costo real si algún día el transporte importa.
- **HTTP en la red interna sin cifrado.** Aceptado: la red `mired` es interna y el aislamiento se hace por red,
  no por TLS (ver ADR-0005).