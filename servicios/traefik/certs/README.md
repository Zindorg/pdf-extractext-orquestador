# Certificados TLS locales

Los certificados no se versionan. Generarlos en cada máquina con [mkcert](https://github.com/FiloSottile/mkcert):

```bash
mkcert -install
mkcert -cert-file cert.pem -key-file key.pem \
  "universidad.localhost" "*.universidad.localhost" \
  "universidad.local" "*.universidad.local"
```

Traefik los lee desde `/etc/certs/cert.pem` y `/etc/certs/key.pem` (ver `traefik/public/config.yml`).
