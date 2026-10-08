# Orquestación

> Contexto de la coordinación del procesamiento de documentos: el Orquestador recibe la solicitud del
> cliente, encadena extracción, resumen opcional e ingesta, y es el único punto de contacto con el resto
> de los servicios.

## Lenguaje

### Documento

- **Definición**: una unidad de contenido que el cliente envía para procesar y que el sistema persiste bajo un
  identificador propio. Es la raíz de todo el modelo: tiene texto extraído, un resumen opcional y un ciclo de
  vida. No es el archivo en sí —el archivo es solo la forma en que llega— ni la petición HTTP que lo trajo.
- **Evitar**: archivo, pdf, request, upload

### Identificador de documento

- **Definición**: el UUID que el Orquestador genera al admitir una ingesta y que nombra al documento en todas
  partes: la respuesta, el header, los eventos y la API de lectura. Es una identidad única y no se reutiliza
  nunca, ni siquiera después de borrar el documento. Identifica **una petición de ingesta**, no a la persona ni
  al cliente: si el mismo cliente manda el mismo contenido dos veces, hay dos identificadores.
- **Evitar**: correlation_id, request_id, trace_id, id

### Texto extraído

- **Definición**: el texto plano que el Extractor obtiene del documento. Es la única representación del
  contenido que el sistema conserva: el archivo original no se guarda. Es también la entrada del Resumidor y
  el material del que se calcula el checksum.
- **Evitar**: contenido, body, output, resultado

### Resumen

- **Definición**: una síntesis en lenguaje natural del texto extraído, generada por el Resumidor. Es opcional:
  el cliente decide si lo pide en la misma solicitud que envía el documento. Ausente significa "nunca se pidió"
  o "se pidió y falló", y esos dos casos se distinguen por el estado de resumen, nunca por la ausencia del
  campo.
- **Evitar**: summary, resumen_ia, tl;dr

### Checksum

- **Definición**: el `sha256` del texto extraído, calculado por el Orquestador y enviado en el evento de
  ingesta. Es la **identidad del contenido**, distinta del identificador del documento: dos documentos con el
  mismo checksum son duplicados aunque tengan identificadores distintos. Es lo que hace que un reintento del
  cliente sea detectable.
- **Evitar**: hash del archivo, md5, fingerprint

### Duplicado

- **Definición**: un documento cuyo checksum ya está ocupado por otro documento activo. No es un error: es una
  entrega legítima que el sistema reconoce y descarta en silencio, porque el contenido ya está guardado. El
  duplicado es una propiedad del contenido, no un fallo de la solicitud.
- **Evitar**: repetido, conflicto, clon

### Estado de entrega

- **Definición**: en qué punto del camino está el documento respecto de haber sido persistido. Es un eje
  propio, independiente del resumen: un documento puede estar entregado y sin resumen a la vez.
  Valores: `INGESTED` (publicado en el stream y pendiente de que el Persistidor lo asiente), `AVAILABLE`
  (ya consultable por la API).
- **Evitar**: status, estado, estado del documento

### Estado de resumen

- **Definición**: si el resumen fue pedido y qué le pasó. Es un eje propio y su valor lo decide **el
  Orquestador**, que es el único que sabe si el cliente lo pidió; el Persistidor no lo deduce ni lo calcula.
  Valores: `NOT_REQUESTED` (el cliente no lo pidió), `PENDING` (se pidió y se está resolviendo), `RESOLVED`
  (se resolvió y está publicado), `FAILED` (se pidió y el Resumidor no respondió).

  **"Estado único de resumen"** (ADR-0006) significa fuente única de ese valor, no que exista un solo estado.
  El Persistidor guarda internamente un `Status` único de dos valores (`PENDING` / `COMPLETED`): es una
  representación de almacenamiento, no el vocabulario de la API, y no reemplaza a este eje.
- **Evitar**: status, estado

### Ingesta

- **Definición**: la incorporación de un documento al sistema, que ocurre cuando el Orquestador publica el
  texto extraído en el stream y el Persistidor lo asienta. Es asíncrona por construcción: la respuesta al
  cliente no espera a que el documento sea consultable. Ingestar no es escribir un archivo ni guardar un
  resultado, es hacer que el documento exista en el sistema.
- **Evitar**: guardado, persistencia, escritura

### Documento abandonado

- **Definición**: un resumen que llegó al sistema sin que existiera el documento al que pertenece —porque el
  evento de ingesta se perdió, se retrasó más de lo que el consumidor reintenta, o el documento nunca llegó a
  publicarse. No es un documento huérfano: el documento puede existir perfectamente y ser el resumen el que no
  tiene a quién pertenece. Lo detecta el Persistidor, que es el consumidor; reintenta con espera creciente y,
  al agotar los intentos, lo aparta a la cola de mensajes rechazados.
- **Evitar**: huérfano, zombie, colgado, resumen huérfano

### Zero-Disk

- **Definición**: la regla de que ningún servicio del ecosistema escribe un documento en su sistema de
  archivos. El contenido se recibe, se transforma y se transmite; donde existe en el proceso, existe como bytes
  en RAM y desaparece al terminar la solicitud. Ningún servicio guarda copias locales, ni temporales, ni de
  recuperación. El único almacenamiento es el del Persistidor.
- **Evitar**: temporal, efímero, en memoria

## Relación entre los dos ejes de estado

`Estado de entrega` y `Estado de resumen` se mueven en escalas distintas y **no se derivan uno del otro**. Un
documento puede estar `INGESTED` con el resumen en cualquiera de los cuatro estados, y un documento `AVAILABLE`
puede haber sido solicitado con `summarize=false` y quedarse en `NOT_REQUESTED` para siempre.

La API de lectura expone los dos ejes por separado precisamente para que el cliente no tenga que inferir. En
cambio, el `Status` único que el Persistidor guarda (`PENDING` / `COMPLETED`) responde a otra pregunta: si el
documento tiene su resumen presente o no. No es contradicción con este glosario: son dos preguntas distintas,
con dos consumidoras distintas.
