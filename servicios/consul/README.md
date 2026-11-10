# Consul

Registro de servicios y KV del sistema. Un solo servidor en la red `mired`; la API y la UI se publican
solo en `127.0.0.1:8500` del host.

## Levantar

```bash
cp config/acl-tokens.json.example config/acl-tokens.json   # completar con: uuidgen
docker network create mired   # si no existe
docker compose up -d
```

`config/acl-tokens.json` y `data/` están en `.gitignore`: el token de management no se versiona. Consul
carga todos los `.json` de `config/`, así que el token se suma a `consul.json` sin tocarlo.

Con `default_policy: allow` las lecturas y los registros funcionan sin token. Para escribir con el token
de management: `-H "X-Consul-Token: $CONSUL_HTTP_TOKEN"`.

## Servicios

```bash
# Catálogo
curl -s http://localhost:8500/v1/catalog/services | python3 -m json.tool

# Instancias sanas de un servicio
curl -s "http://localhost:8500/v1/health/service/pdf-extractext-orquestador?passing" | python3 -m json.tool

# Servicios y checks del agente
curl -s http://localhost:8500/v1/agent/services | python3 -m json.tool
curl -s http://localhost:8500/v1/agent/checks | python3 -m json.tool

# Dar de baja una instancia
curl -X PUT http://localhost:8500/v1/agent/service/deregister/<service-id>
```

### Registrar un servicio

El check lo ejecuta **el contenedor de Consul**: `localhost` ahí es Consul mismo. La URL del check tiene
que usar el nombre del servicio en `mired` (o su IP), igual que `Address`.

```bash
curl -X PUT http://localhost:8500/v1/agent/service/register \
  -H "Content-Type: application/json" \
  -d '{
    "ID": "microservicio-1",
    "Name": "microservicio",
    "Address": "microservicio",
    "Port": 5000,
    "Check": {
      "HTTP": "http://microservicio:5000/health",
      "Interval": "10s",
      "Timeout": "2s",
      "DeregisterCriticalServiceAfter": "1m"
    }
  }'
```

El orquestador se registra solo al arrancar y se da de baja al apagarse (ver `CONSUL_HTTP_ADDR` en el
`.env.example` de la raíz). El enrutamiento HTTP lo hace Traefik por labels de Docker: no hace falta
agregar tags `traefik.*` en el registro.

## KV

| Operación | Método | Endpoint |
|---|---|---|
| Leer | `GET` | `/v1/kv/:key` (`?raw` = valor sin base64) |
| Listar bajo un prefijo | `GET` | `/v1/kv/:prefix?recurse` (`?keys` = solo nombres) |
| Crear/actualizar | `PUT` | `/v1/kv/:key` |
| Eliminar | `DELETE` | `/v1/kv/:key` (`?recurse` = todo el prefijo) |

```bash
curl -X PUT -d 'Hola Mundo' http://localhost:8500/v1/kv/saludo
curl -s "http://localhost:8500/v1/kv/saludo?raw"
```

Sin `?raw`, `Value` viene en base64. Documentación completa:
https://developer.hashicorp.com/consul/api-docs/kv

## Pendiente

- Rotar el `initial_management` que estuvo versionado en `consul.json`: quedó en el historial de git.
- Pasar `default_policy` a `deny` con tokens por servicio (el orquestador solo necesita
  `service "pdf-extractext-orquestador" { policy = "write" }`).
