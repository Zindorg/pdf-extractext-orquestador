# Traefik solo en el borde público

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

El punto de partida concreto es que las tres referencias del blueprint son contradictorias entre sí: describen
`internal-net` en el punto de entrada público y también en la red interna, lo que hace que el mismo servicio
sea alcanzable de dos maneras con las mismas reglas.

## Decisión

**Traefik queda solo en el borde público. El servicio es el dueño de su API: define sus rutas, sus prefijos
`/api/v1`, sus códigos de error y sus headers.**

Traefik hace cuatro cosas y ninguna más:

1. termina TLS;
2. enruta por host y por path;
3. agrega cabeceras de enrutamiento;
4. agrega límites de tasa y de tamaño del cuerpo.

Y **no** hace cinco cosas que el blueprint le pedía: no reescribe `/api/v1`, no normaliza nombres de headers,
no decide códigos de error, no aplica reglas de negocio y no guarda estado.

Corolario operativo: **el puerto interno del servicio no se expone fuera de la red interna.** Es lo que hace
que las reglas valgan. Si el puerto queda publicado, el proxy deja de ser el único camino y el ADR queda sin
respaldo.

## Alternativas consideradas

**Traefik como dueño de las rutas.** Rechazada. Un proxy que decide el prefijo tiene que conocer la
estructura interna del servicio para enrutar bien, así que conoce la API de todas formas; la diferencia es que
ahora la duplica en dos lugares y la documenta en un tercero.

**Proxy transparente, sin reescritura de prefijo.** Es la alternativa elegida. Se conoce y se nombra aparte
porque es la que rompe el patrón: el blueprint original reescribe `/api/v1` hacia una ruta interna sin el
prefijo, y por eso las rutas internas son distintas de las que ve el cliente.

**Quitar Traefik y exponer el servicio directamente.** Rechazada. El MVP es público y hacen falta TLS,
enrutado y límites de tasa; esos límites son la primera línea de defensa de un servicio anónimo (ADR-0007).

**Mantener las dos redes con reglas duplicadas.** Rechazada por la contradicción de partida: dos caminos de
entrada con las mismas reglas significa que toda regla nueva hay que aplicarla dos veces, y la que se olvide
deja una puerta abierta. Se conserva el término en el registro histórico (ADR-0007 remite a él) y se retira de
la arquitectura.

## Consecuencias

### Positivas

- **Las rutas del cliente y las del servicio son las mismas.** No hay un prefijo público y otro interno que
  mantener sincronizados.
- **El proxy no duplica reglas de negocio.** Las validaciones y los códigos de error viven en el código, que es
  donde se pueden probar.
- **Los headers de la API los define el servicio**, así que la documentación del contrato y el comportamiento
  real no pueden divergir.
- **El puerto interno no se expone**, y por eso el cliente no puede saltarse el proxy y sus límites.

### Negativas / costo

- **Cambiar la ruta en el cliente no se puede esconder en el proxy.** Si mañana el prefijo cambia, hay que tocar
  el cliente y el servicio; el proxy no lo puede absorber por su cuenta.
- **El servicio tiene que conocer su propio path público**, porque es el dueño de la API. Si se despliega detrás
  de un path distinto, hay que reconfigurarlo.
- **Los límites del proxy hay que replicarlos en la aplicación** para que el comportamiento sea el mismo en
  pruebas: un límite que solo existe en Traefik no se puede testear sin levantar el proxy.
