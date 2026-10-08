# Identidad única: document_id

## Contexto

Un sistema con tres procesos detrás de un balanceador y una cola de mensajes en el medio tiene, por lo menos,
tres cosas que podrían ser "el identificador de esta petición": el identificador que el balanceador o el proxy
asigna, el identificador que el servicio genera para sus logs, y el identificador del documento en la base.

El patrón habitual en microservicios es tener **varios**: un `X-Correlation-ID` que viaja entre servicios para
enlazar logs, un `request_id` interno, y un `id` de recurso en la base. Se agrega cada uno cuando aparece el
problema que resuelve, y el resultado es que cada servicio tiene su propio vocabulario y que enlazar un log de
Traefik con uno del extractor requiere saber el mapa.

Acá hay una presión para no hacerlo así, y es que **el sistema ya tiene un identificador de negocio que es
obligatorio**: el documento. No es un `id` técnico, es una identidad que el cliente recibe, que el Persistidor
indexa, y que vuelve a aparecer en cada evento. Si además de ese identificador hay otros dos, entonces el
nombre del documento pasa a ser uno más y hay que responder "cuál de los tres".

El otro punto de presión es más concreto: **el sistema no tiene autenticación** (ver ADR-0007). Sin identidad del
cliente, no hay nadie a quien atribuir una ingesta. Lo que sí se puede atribuir es la petición concreta.

## Decisión

**Un solo identificador de punta a punta: `document_id`, un UUID v4 que genera el Orquestador al admitir la
petición. El header se llama `X-Document-Id`.**

No existe un identificador de correlación separado. Si hace falta enlazar logs, el `document_id` sirve para eso
exactamente, y el log de acceso lo lleva en el mismo campo que todo lo demás.

### El `document_id` y el checksum responden a preguntas distintas

Esta es la parte que conviene no resumir, porque es la que hace que el sistema se comporte bien ante un
reintento:

- El **`document_id` identifica una petición de ingesta.** Se genera una vez, cuando el cliente manda el
  documento, y nombra a ese intento para siempre. Si el cliente manda el mismo contenido dos veces, hay dos
  `document_id`. Un `document_id` **no se reutiliza nunca**, ni siquiera después de borrar el documento: es un
  identificador, no un espacio de nombres.
- El **checksum identifica el contenido** (ver ADR-0003). Dos `document_id` con el mismo checksum son el mismo
  contenido, y el segundo se descarta.

La consecuencia práctica: **el `document_id` no es reintentable y el checksum sí.** Cuando el cliente pierde la
respuesta y vuelve a subir, obtiene un `document_id` nuevo —que es lo correcto, porque es una petición nueva— y
la deduplicación la resuelve el checksum. Confundir los dos es un error de diseño que cuesta caro: si el
`document_id` fuera derivado del contenido, dos subidas simultáneas del mismo archivo entrarían en carrera por
el mismo identificador y una tendría que esperar a la otra.

El `document_id` viaja en la respuesta `202`, en el header `X-Document-Id` de todas las respuestas de la
petición, en el evento `original`, en el evento `summary_resolved`, y es el parámetro de todas las rutas de
lectura y borrado. No hay un solo lugar del sistema donde el documento no se pueda nombrar con esta palabra.

## Alternativas consideradas

**`X-Correlation-ID` genérico, como proponía el blueprint original.** Rechazada. Deja dos identificadores vivos
en el mismo sistema y obliga a un mapa mental para pasar del log de una petición al documento que la produjo. Es
el caso clásico: el identificador de correlación se agrega "por si acaso", y una vez que está en tres servicios
nadie lo saca.

**`X-Request-ID`, como usa hoy el blueprint del Persistidor.** Rechazada por lo mismo que el anterior, y además
introduce una contradicción entre repositorios desde el primer día: si el Persistidor nombra `X-Request-ID` y el
Orquestador nombra `X-Document-Id`, cada uno tiene su vocabulario y el que lee los logs de los dos tiene que
saber qué son. El cambio de nombre que hace falta en el Persistidor es una fila del delta
(`docs/contracts/delta-persister.md`) y no un problema de diseño que se arrastra.

**Que el cliente provea el identificador.** Rechazada por dos razones, y la segunda es la importante. La primera
es que un identificador de documento no es un dato de negocio que el cliente deba conocer antes de que exista el
documento. La segunda es que un cliente puede reenviar el mismo identificador por error o a propósito, y entonces
dos ingesta distintas compiten por un nombre: el cliente decide cuál gana. Además, sin autenticación, un
identificador que el cliente elige no distingue "el cliente reintentó" de "alguien más lo está usando".

**Que el Persistidor genere el `document_id`.** Rechazada por una razón estructural: el `document_id` tiene que
volver en la respuesta `202` al cliente, que se emite **antes** de que exista el documento en la base. Si el
identificador lo genera el consumidor, no hay a quién preguntar.

**Generarlo arriba de la base, como el `ObjectId` de MongoDB.** Rechazada. El dominio no debe conocer el motor
de almacenamiento que resulta que se está usando. Es además una identidad interna con formato propio de Mongo
expuesta por la API pública, que es exactamente el acoplamiento que la arquitectura hexagonal existe para
evitar.

## Consecuencias

### Positivas

- **Una sola palabra para nombrar un documento en todo el sistema.** `document_id` es la que aparece en la
  respuesta, en los logs, en los eventos y en las rutas. No hay un segundo nombre que aprender ni un mapa que
  consultar.
- **Los logs son correlacionables de punta a punta** con el identificador que el cliente ya tiene en la mano. Si
  el cliente reporta un problema, el `document_id` alcanza para encontrar la extracción, el resumen y la
  publicación de esa ingesta.
- **El sistema no necesita una tabla de correspondencias** entre "el `request_id` que me dio el balanceador" y
  "el `document_id` que está en la base".
- **La deduplicación sobrevive al reintento**, porque vive en el checksum y el checksum no depende del
  identificador.
- **Cambiar el nombre del header es un cambio aislado**, no una migración: son cuatro archivos y una fila en el
  delta cross-repo.

### Negativas / costo

- **No hay identidad del cliente.** Sin autenticación (ADR-0007), un `document_id` no dice quién subió el
  documento, ni permite restringir el acceso a un documento que el cliente haya subido. Cualquiera que tenga la
  URL y el `document_id` puede leerlo y borrarlo. Es una consecuencia deliberada del MVP, no un descuido, y se
  resuelve cuando se agregue autenticación: el `document_id` no cambia.
- **El `document_id` no es reintentable.** Un cliente que pierde la respuesta no puede "reintentar con el mismo
  `document_id`"; reintenta con uno nuevo y deja que el checksum resuelva. Es el comportamiento correcto, pero
  no es intuitivo y hay que documentarlo en el contrato público, porque un cliente bien diseñado va a querer
  idempotencia por clave.
- **Borrar no libera el `document_id`.** Un documento borrado conserva el suyo para siempre. Si ese nombre vuelve
  a aparecer en una ingesta, hay un problema de datos o un ataque, y las dos cosas son graves.
- **Cambiar el nombre del header es una ruptura de contrato público.** Cualquier cliente que ya lea
  `X-Request-ID` deja de funcionar. Es barato hoy, con cero clientes en producción, y caro dentro de un año.
  Por eso se decide ahora y no después.