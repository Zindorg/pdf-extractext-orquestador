# Traefik en el borde público y en la red interna

> **Reemplaza a la primera versión de este ADR**, que decidía un único Traefik en el borde público. La
> decisión de fondo se conserva —**el proxy es un proxy, no el dueño de la API**— y se agrega una segunda
> instancia para el enrutamiento interno. El cambio está registrado en §Registro del cambio.

## Contexto

El blueprint original coloca un proxy inverso (Traefik) entre el mundo exterior y el servicio. Eso trae una
pregunta que hay que responder antes de escribir los contratos: **¿el proxy es el dueño de la API o solo un
proxy?**

La diferencia parece de detalle y no lo es. Si Traefik es el dueño, entonces:

- las rutas, los prefijos y los nombres de los headers son decisiones suyas, y la documentación de la API
  miente si no los replica;
- un cliente puede alcanzar el servicio por dos caminos distintos —el proxy y la ruta interna— y si cada uno
  enrutiza distinto, hay dos APIs;
- las validaciones, los topes y el manejo de errores quedan partidos entre el proxy y el código.

Si el proxy es solo un proxy, entonces reenvía sin interpretar, y el servicio sigue siendo el dueño de su
propia API.

A esa pregunta se agrega una segunda, que la primera versión del ADR no se hacía: **entre el Orquestador y
los workers, ¿el enrutamiento lo resuelve el DNS de Docker o una pieza propia?** El blueprint asumía que el
nombre del contenedor (`http://extractor:8000`) alcanzaba. Ese nombre es estable mientras el contenedor
exista, pero acopla cada cliente a la topología del compose y deja sin un lugar único las políticas internas
(límites, log de acceso, cortes). Cuando los workers todavía no tienen repositorio, esa es toda la integración
que hay: un nombre que cada servicio tiene que conocer y mantener.

El punto de partida concreto sigue siendo el mismo: las referencias del blueprint se contradicen entre sí y
describen `internal-net` en el punto de entrada público y también en la red interna, lo que hace que el mismo
servicio sea alcanzable de dos maneras con las mismas reglas.

## Decisión

**Dos instancias de Traefik, con roles separados, y en ninguna el proxy es dueño de la API.**

`traefik-public` vive en el borde, sobre `public-net`, y hace cuatro cosas y ninguna más:

1. termina TLS;
2. enruta por host y por path (`Host(...) && PathPrefix(/api/v1)`);
3. agrega cabeceras de enrutamiento;
4. agrega límites de tasa y de tamaño del cuerpo.

`traefik-internal` vive en `mired` y enruta el tráfico del Orquestador hacia los workers por hostname interno:
`extractor.internal`, `summarizer.internal`, `persister.internal`. El Orquestador llama a esas URL como si
fueran servicios; el nombre resuelve a `traefik-internal` (por alias de red en `mired`) y este enruta por
`Host`. Los workers se registran por sus labels desde sus propios repos.

Ninguna de las dos instancias reescribe el prefijo `/api/v1`, normaliza names de headers, decide códigos de
error, aplica reglas de negocio ni guarda estado. **El servicio sigue siendo el dueño de su API.**

Corolario operativo: **el puerto HTTP del Orquestador no se publica al host.** Sólo `traefik-public` publica
`80`/`443`. Si el puerto interno queda publicado, el borde deja de ser el único camino y este ADR queda sin
respaldo.

## Registro del cambio

La versión anterior decidía **un solo Traefik, solo en el borde público**, y empujaba el tráfico interno al
DNS de servicio (`http://extractor:8000`, sin prefijo). Lo que cambió:

- **Se agrega `traefik-internal` en `mired`.** El enrutamiento interno deja el DNS de contenedor y pasa por
  una pieza propia, con hostnames estables (`*.internal`) desacoplados del nombre del compose.
- **Se conserva** la tesis de propiedad de la API, los cuatro trabajos del borde y el corolario de no exponer
  el puerto interno.
- **La alternativa "mantener las dos redes con reglas duplicadas"** ya no se rechaza por sí sola: lo que se
  rechazó en la versión anterior era **duplicar las mismas reglas de entrada**; lo que se agrega ahora es un
  rol **distinto** (routing interno), no una segunda puerta pública idéntica.

## Alternativas consideradas

**Traefik como dueño de las rutas.** Rechazada. Un proxy que decide el prefijo tiene que conocer la
estructura interna del servicio para enrutar bien, así que conoce la API de todas formas; la diferencia es que
ahora la duplica en dos lugares y la documenta en un tercero.

**Proxy transparente, sin reescritura de prefijo.** Es la alternativa elegida. El blueprint original
reescribe `/api/v1` hacia una ruta interna sin el prefijo, y por eso las rutas internas son distintas de las
que ve el cliente. Acá el Persistidor sigue montado en `/api/v1` y el proxy no toca el string.

**Quitar Traefik y exponer el servicio directamente.** Rechazada. El MVP es público y hacen falta TLS,
enrutado y límites de tasa; esos límites son la primera línea de defensa de un servicio anónimo (ADR-0007).

**Enrutamiento interno por DNS de servicio, sin Traefik interno.** Rechazada. Era la decisión de la versión
anterior. Acopla cada cliente al nombre del contenedor y reparte las políticas internas entre los llamadores.
Una instancia interna las centraliza sin exponer nada al host.

## Consecuencias

### Positivas

- **Las rutas del cliente y las del servicio son las mismas.** No hay un prefijo público y otro interno que
  mantener sincronizados.
- **El proxy no duplica reglas de negocio.** Las validaciones y los códigos de error viven en el código, que es
  donde se pueden probar.
- **Los headers de la API los define el servicio**, así que la documentación del contrato y el comportamiento
  real no pueden divergir.
- **El puerto interno no se expone.** El cliente no puede saltarse `traefik-public` y sus límites.
- **El enrutamiento interno es una decisión local del ecosistema**: cambiar de dónde vive un worker no toca el
  código del Orquestador, sólo el label del worker.

### Negativas / costo

- **Dos proxies que operar.** Hay que desplegar, versionar y diagnosticar `traefik-public` y
  `traefik-internal`, con sus dos configuraciones.
- **Un salto más y un modo de falla más en la red interna.** Si `traefik-internal` está mal configurado o
  caído, el Orquestador no alcanza a los workers aunque estos estén sanos. El `/ready` del ecosistema debería
  cubrirlo.
- **`traefik-internal` tiene que estar en `mired`** y ver los contenedores por el provider de Docker para
  enrutarlos; no alcanza con declararlo en el compose del Orquestador.
- **Cambiar la ruta en el cliente no se puede esconder en el proxy.** Si mañana el prefijo cambia, hay que tocar
  el cliente y el servicio; el proxy no lo puede absorber por su cuenta.
- **Los límites del proxy hay que replicarlos en la aplicación** para que el comportamiento sea el mismo en
  pruebas: un límite que solo existe en Traefik no se puede testear sin levantar el proxy.
