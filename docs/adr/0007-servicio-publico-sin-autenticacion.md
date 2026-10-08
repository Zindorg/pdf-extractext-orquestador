# Servicio público sin autenticación

## Contexto

El Orquestador expone un endpoint público: cualquiera que conozca la URL puede subir un documento y pedir que se
le extraiga texto y, opcionalmente, un resumen. La pregunta es si eso es aceptable en esta fase. La respuesta
honesta es que no lo sería en producción abierta, pero la pregunta útil no es "¿debería haber autenticación?",
porque la respuesta obvia es sí y no informa nada. La pregunta útil es **qué se puede hacer sin ella, qué riesgo
queda asumido y quién lo cierra**.

Hay tres restricciones reales que empujan a no implementarla ahora:

**El servicio es público por diseño, no por descuido.** Extrae texto de documentos y no tiene cuentas de
usuario, ni sesión, ni registro. Agregar autenticación convertiría un servicio que se despliega como un
contenedor en otro que necesita un emisor de tokens, un secreto compartido y un camino de renovación. Eso no es
un detalle de configuración: es una dependencia de infraestructura nueva.

**La arquitectura no tiene dónde ponerla todavía.** La autenticación sería un middleware en
`internal/api/middleware/`, junto a `concurrency.go` y al de logging. Pero una autenticación real necesita un
secreto, y para un servicio anónimo eso implica decidir formato de token, algoritmo de firma, duración y
rotación. Ninguna de esas decisiones está tomada, y tomarlas ahora sería diseño especulativo: se harían sin
clientes reales que las ejerzan.

**La lectura ya no la exponemos nosotros.** El endpoint de lectura pertenece al Persistidor
(`ARCHITECTURE_REPOSITORIES.md`), no al Orquestador. La superficie anónima propia del MVP es una: admitir
documentos y pedir un resumen de un documento existente.

## Decisión

**El MVP no tiene autenticación.** No hay `Authorization`, ni API keys, ni sesiones, ni cookies, ni ningún
mecanismo de identidad del cliente. Cualquiera con la URL puede usar el servicio.

El ADR existe sobre todo para dejar constancia de que **el riesgo está identificado y asumido a propósito**, no
para justificar la omisión. Un sistema anónimo que expone documentos ajenos es un riesgo conocido; uno que
funciona sin que nadie haya evaluado el riesgo es un descuido.

Lo que sí se implementa ahora son cuatro mitigaciones. Ninguna es autenticación, y conviene no confundirlas
con ella: acotan el daño de un abuso, pero no impiden que un tercero use el servicio.

- **Topes de concurrencia por réplica.** El extractor y el resumidor tienen semáforos que devuelven `429` al
  superarse, en `internal/api/middleware/concurrency.go`. Acotan el trabajo concurrente por réplica.
- **Límite de tamaño del documento en el borde.** Un documento demasiado grande se rechaza en la puerta 1
  antes de consumir memoria y CPU.
- **Zero-Disk.** El contenido vive en RAM durante la petición y desaparece al terminar. Un abuso no deja rastro
  en el sistema de archivos, lo que acota el daño a memoria y CPU.
- **Ventana de ejecución acotada.** El resumen tiene un timeout deliberadamente holgado
  (`internal/adapters/http_summarizer/client.go`) porque los modelos serializan lento, pero está acotado: una
  subida no puede retener recursos indefinidamente.

Los topes de concurrencia son **por réplica**, no globales. El agregado multiplica el total, así que un
deployment con muchas réplicas admite proporcionalmente más. Es la misma decisión de ADR-0001 aplicada a otro
recurso.

## Alternativas consideradas

**Autenticación por API key desde el MVP.** Rechazada. Sería más simple que los tokens, pero una API key sigue
siendo un secreto que hay que emitir, entregar, rotar y revocar: el mismo trabajo de infraestructura que se
quería evitar. Y sin identidad detrás, no responde "¿de quién es este documento?", que es la pregunta que de
verdad importa.

**Autenticación diferida a una fase posterior, sin nombrarla.** Rechazada por el modo de fallo que produce:
"agregamos autenticación más adelante", sin un ADR que la nombre y sin nadie a cargo, se convierte en algo que
nadie hace. Nombrarla acá la deja pendiente en lugar de olvidada.

**Confiar en que el servicio va a quedar detrás de un gateway que autentica.** Rechazada. Supone una cosa que
nadie sabe con certeza y que el diseño no debe dar por hecha: si la autenticación depende de una pieza externa, es una
dependencia, y las dependencias se nombran. Este ADR no dice que el gateway autentique; dice que el Orquestador
no lo hace, y por lo tanto un gateway que autentique es una decisión de despliegue, no del servicio.

**Exponer el servicio solo en la red interna.** Incompatible con el objetivo del producto y con ADR-0005: el
cliente es externo. Se documenta acá solo para dejar claro que la alternativa existe y por qué no aplica.

## Consecuencias

### Positivas

- **Despliegue de un contenedor, sin infraestructura de identidad.** Es lo que hace que el MVP sea desplegable
  sin un emisor de tokens ni un secreto que rotar.
- **Superficie de ataque pequeña y conocida.** Una sola ruta anónima propia, más la de resumen a demanda.
- **Las mitigaciones ya están en el código**, no en un ticket futuro: topes por réplica, límite de tamaño,
  Zero-Disk y timeout.
- **Agregar autenticación después no rompe el contrato.** El `document_id` y las rutas quedan igual; se agrega un
  middleware. Ninguna decisión de este ADR hay que revertir.

### Negativas / costo

- **Un tercero puede subir documentos y consumir CPU y memoria** sin ninguna restricción de identidad. Los topes
  por réplica acotan el daño, no lo impiden, y con varias réplicas el límite total es mayor.
- **No se puede atribuir una ingesta a nadie.** El sistema no sabe quién subió qué. Sin eso no hay forma de
  detectar un cliente abusivo ni de publicar métricas por cliente.
- **No hay aislamiento entre clientes.** Cuando exista más de un cliente, uno puede leer el documento de otro
  usando el `document_id` (ADR-0004).
- **El riesgo crece con el tiempo sin que nada lo avise.** Este ADR no vence solo; hay que revisarlo antes de
  exponer el servicio fuera de un entorno controlado.
