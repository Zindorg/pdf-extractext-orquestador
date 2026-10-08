# Checksum calculado por el productor

## Contexto

El sistema deduplica contenido: si un cliente manda dos veces el mismo documento, el sistema debería reconocerlo
y no guardar dos veces lo mismo. Para eso hace falta una huella del contenido, y la pregunta es quién la calcula.

El candidato obvio es el Persistidor, porque es el que tiene la base de datos y es donde la deduplicación
importa. Pero hay un detalle que lo descarta: **el Persistidor nunca ve el documento original.** Solo recibe
el texto ya extraído, en el evento de ingesta. Así que "el checksum del archivo" no es una opción para él
aunque quisiera, y esto no es un detalle: es que el sistema entero cumple Zero-Disk, el archivo no se guarda en
ningún lado, y el texto extraído es la única representación del contenido que existe.

Queda entonces la pregunta de si el productor —el Orquestador— o el consumidor calcula la huella del texto. La
diferencia no es dónde se ejecuta una función: es **quién es la fuente de verdad** cuando los dos lados no
coinciden.

Y hay una asimetría que empuja la decisión: el checksum se usa para deduplicar, y la deduplicación ocurre
**asíncronamente**, en el consumidor, cuando procesa el evento. Si el productor y el consumidor calcularan el
valor por separado, existiría una ventana —la que media entre el `XADD` y el procesamiento— en la que ambos
tienen un valor y todavía no se compararon.

## Decisión

**El Orquestador calcula el checksum y es su fuente de verdad. El Persistidor lo persiste tal cual, sin
recalcularlo nunca.**

Concretamente: `checksum = sha256(texto extraído)`, en minúsculas, en hexadecimal. Se calcula en el paso 5 del
pipeline, después de que la puerta 2 confirmó que hay texto y antes de que se publique nada. Viaja en el evento
`original` y el Persistidor lo guarda como un string cualquiera, sin ninguna lógica de verificación.

El detalle de *minúsculas y hexadecimal* no es cosmético: es lo que hace que el mismo texto produzca la misma
cadena en cualquier implementación. Sin normalizar, un `SHA-256` en mayúsculas y otro en minúsculas producen
dos cadenas distintas para el mismo contenido, y la deduplicación deja de funcionar sin que nada falle de forma
visible. Es el tipo de bug que aparece meses después.

El checksum es **la identidad del contenido**; el `document_id` es la identidad de la petición. No son la misma
cosa y el glosario los separa por esa razón (ver ADR-0004). La garantía fuerte de deduplicación no vive en la
aplicación sino en un índice único parcial sobre el checksum con filtro `deleted_at: null`
(`ARCHITECTURE_REPOSITORIES.md` §8): si dos eventos con el mismo checksum llegan, la base rechaza el segundo
con el código `11000` y el consumidor lo descarta en silencio como duplicado.

Que un documento borrado libere su checksum es parte de esta decisión, no un efecto secundario: es lo que
permite que el mismo contenido vuelva a ingerirse más tarde, con un `document_id` nuevo.

## Alternativas consideradas

**Calcularlo el consumidor.** Rechazada por la ventana de discrepancia. Entre el `XADD` y el procesamiento del
mensaje, el valor existe en dos lugares y no se ha comparado. Además, "el consumidor calcula el checksum" tiene
un costo oculto: para calcularlo necesita el texto completo en memoria otra vez, o el binario, y la razón por la
que el productor lo calcula es justamente que es el único momento en que el texto está disponible sin haberlo
guardado antes.

**Hashear el documento binario.** Rechazada por dos razones independientes, y vale la pena separarlas porque
fallan de forma distinta. La primera es que el Persistidor nunca ve el binario: no puede hacerlo. La segunda, y
más importante, es que **no deduplicaría lo que queremos deduplicar**: el mismo texto extraído desde dos PDFs
distintos —el mismo contrato guardado dos veces con distinto tamaño de página, o la misma hoja de cálculo
exportada en dos formatos— produce archivos distintos y checksums distintos. Se guardarían dos veces, que es
justo lo que la deduplicación debía evitar. El texto extraído es el contenido; el archivo es solo el vehículo.

**Usar MD5.** Rechazado. No por velocidad, que es irrelevante acá, sino porque el estilo de colisión
práctica de MD5 sigue siendo un problema abierto y el costo de usar SHA-256 es cero. La decisión de no elegir un
algoritmo débil se toma una vez y no cuesta nada después.

**Un checksum compuesto de nombre, tamaño y fecha.** Rechazado. Es una identidad de *archivo* disfrazada: dos
copias del mismo documento con nombres distintos produce dos registros, que es el problema original.

## Consecuencias

### Positivas

- **Una sola fuente de verdad.** El valor que el productor calculó es el que queda, y el consumidor no puede
  discrepar. No hay reconciliación que hacer ni un mensaje de "el checksum no coincide".
- **La deduplicación real está en el índice, no en la aplicación.** Una carrera entre dos eventos con el mismo
  checksum se resuelve en la base de datos, que es donde no puede haber carrera. La aplicación no necesita un
  read-then-write para ser consistente.
- **Normalizado y comparable entre repos.** El mismo texto da el mismo checksum en cualquier implementación, lo
  que permite escribir herramientas de verificación en cualquier lenguaje.
- **Borrar libera el checksum**, así que el mismo contenido puede volver a ingerirse con un `document_id`
  distinto, que es lo que un cliente espera después de borrar algo.
- El consumidor no necesita el texto dos veces, ni el binario, ni ninguna memoria extra para deduplicar.

### Negativas / costo

- **El checksum depende de que la extracción sea determinista.** Si el Extractor devuelve texto ligeramente
  distinto para el mismo documento —una versión distinta de la librería de parsing, un OCR no determinista, un
  salto de línea que se normalizó—, el checksum cambia. La deduplicación falla en silencio: el cliente reenvía
  lo que cree que es el mismo documento y el sistema lo guarda como nuevo.
- **Cambiar el Extractor invalida los checksums guardados.** Es la consecuencia más importante de esta decisión
  y conviene tenerla presente: no es que los datos se corrompan, es que la relación "esto es lo mismo que aquello"
  deja de ser cierta para todo el histórico. Después de un cambio en el pipeline de extracción, **re-ingerir un
  PDF "idéntico" produce un documento nuevo en vez de un duplicado**, y el cliente que lo estaba probando ve que
  el sistema duplicó en lugar de deduplicar. Cualquier migración de este tipo tiene que decidir qué hacer con
  el histórico y decírselo al cliente.
- **No se puede verificar sin el texto.** El checksum no se puede recalcular a partir de nada que el
  Persistidor tenga guardado aparte del propio texto, así que una corrupción silenciosa del texto guardado no se
  detecta comparando: hay que recalcular desde el evento original, que ya no está.
- **Un cambio de algoritmo es una ruptura.** Pasar de SHA-256 a otra cosa invalida todos los checksums, igual que
  cambiar el Extractor. Por eso la elección del algoritmo y de su formato de salida se fija acá y no se toca por
  mejoría de rendimiento.