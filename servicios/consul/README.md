### Listar servicios
```bash
curl http://localhost:8500/v1/catalog/services | python3 -m json.tool

# Ver health del servicio
curl http://localhost:8500/v1/health/service/microservicio | python3 -m json.tool
```

### JSON
```json
{
   "service": {
     "name": "microservicio",
     "port": 8080,
     "check": {
       "id": "microservicio-http",
       "name": "microservicio HTTP health check",
       "http": "http://localhost:8080/health",
       "interval": "10s",
       "timeout": "2s"
     }
   }
 }
```

### Leer una clave (KV)

```bash
# Leer una clave específica (devuelve el valor en base64 por defecto)
curl http://localhost:8500/v1/kv/mi-clave

# Leer con el valor decodificado en texto plano
curl http://localhost:8500/v1/kv/mi-clave?raw

# Leer todas las claves bajo un prefijo (recursivo)
curl http://localhost:8500/v1/kv/mi-prefijo/?recurse

# Leer solo los nombres de las claves (sin valores)
curl http://localhost:8500/v1/kv/mi-prefijo/?keys
```

### Ejemplo práctico de respuesta (sin `?raw`)

```json
[
  {
    "LockIndex": 0,
    "Key": "mi-clave",
    "Flags": 0,
    "Value": "SG9sYSBNdW5kbyE=",
    "CreateIndex": 123,
    "ModifyIndex": 123
  }
]
```

El campo `Value` está en **base64**. Para decodificarlo sin `?raw`:

```bash
curl -s http://localhost:8500/v1/kv/mi-clave | python3 -c "import sys,json,base64; [print(base64.b64decode(i['Value']).decode()) for i in json.load(sys.stdin)]"
```

### Otros endpoints útiles del KV

| Operación | Método | Endpoint |
|---|---|---|
| Leer | `GET` | `/v1/kv/:key` |
| Crear/Actualizar | `PUT` | `/v1/kv/:key` |
| Eliminar | `DELETE` | `/v1/kv/:key` |
| Eliminar recursivo | `DELETE` | `/v1/kv/:prefix?recurse` |

### Crear una clave de ejemplo

```bash
curl -X PUT -d 'Hola Mundo' http://localhost:8500/v1/kv/saludo
```

Si ACL está habilitado con `default_policy: allow` y token `initial_management`, Si necesitas autenticarte, usar el header:

```bash
curl -H "X-Consul-Token: {{token}}" \
  http://localhost:8500/v1/kv/mi-clave?raw


curl -X PUT http://localhost:8500/v1/agent/service/register \
  -H "Content-Type: application/json" \
  -d '{
    "ID": "microservicio-1",
    "Name": "microservicio",
    "Port": 5000,
    "Meta": {
      "traefik.enable": "true",
      "traefik.http.routers.microservicio.rule": "Host(`microservicio.universidad.localhost`)",
      "traefik.http.routers.microservicio.entryPoints": "https",
      "traefik.http.routers.microservicio.tls": "true",
      "traefik.http.services.microservicio.loadbalancer.server.port": "5000"
    },
    "Checks": [
      {
        "Name": "HTTP health",
        "HTTP": "http://localhost:5000/health",
        "Interval": "10s",
        "Timeout": "2s"
      },
      {
        "Name": "TCP port check",
        "TCP": "localhost:5000",
        "Interval": "15s",
        "Timeout": "2s"
      }
    ]
  }'
```

### READ — Listar servicios del agente
```bash
curl http://localhost:8500/v1/agent/services | python3 -m json.tool
```

### READ — Ver un servicio específico
```bash
curl http://localhost:8500/v1/agent/service/microservicio-1 | python3 -m json.tool
```

### READ — Listar servicios del catálogo
```bash
curl http://localhost:8500/v1/catalog/services | python3 -m json.tool
```

### READ — Health de un servicio
```bash
# Todos los nodos del servicio
curl http://localhost:8500/v1/health/service/microservicio | python3 -m json.tool

# Solo nodos con health "passing"
curl "http://localhost:8500/v1/health/service/microservicio?passing" | python3 -m json.tool
```

### READ — Listar checks del agente
```bash
curl http://localhost:8500/v1/agent/checks | python3 -m json.tool
```

### DELETE — Dar de baja un servicio
```bash
curl -X PUT http://localhost:8500/v1/agent/service/deregister/microservicio-1
```

```bash
# Leer una clave específica (devuelve el valor en base64 por defecto)
curl http://localhost:8500/v1/kv/mi-clave

# Leer con el valor decodificado en texto plano
curl http://localhost:8500/v1/kv/mi-clave?raw

# Leer todas las claves bajo un prefijo (recursivo)
curl http://localhost:8500/v1/kv/mi-prefijo/?recurse

# Leer solo los nombres de las claves (sin valores)
curl http://localhost:8500/v1/kv/mi-prefijo/?keys
```

La documentación completa del KV Store está en: https://developer.hashicorp.com/consul/api-docs/kv

```bash
# Crear una clave de ejemplo
curl -X PUT -d 'Hola Mundo' http://localhost:8500/v1/kv/saludo
```
```bash
# Como tienes ACL habilitado con default_policy: allow y token initial_management, no deberías tener problemas de permisos. Si necesitas autenticarte, usa el header:
curl -H "X-Consul-Token: {{token}}" \
  http://localhost:8500/v1/kv/mi-clave?raw
```
