# syntax=docker/dockerfile:1

# Build multi-stage. Go compila estático, así que la imagen final no necesita ni sistema
# operativo completo: distroless static deja solo el binario y los certificados TLS, sin shell ni
# gestor de paquetes, que es la menor superficie posible para un servicio público (ADR-0007).
FROM golang:1.27 AS build

WORKDIR /src

# Copiar los manifiestos primero: mientras no cambien, la capa de descarga de módulos se reutiliza.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

# CGO off para un binario estático, que no arrastre la libc del builder.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server


FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="pdf-extractext-orquestador" \
      org.opencontainers.image.description="Punto de entrada público del sistema PDF-Extractext" \
      org.opencontainers.image.source="https://github.com/Zindorg/pdf-extractext-orquestador"

WORKDIR /app
COPY --from=build /out/server /app/server

# El usuario nonroot del distroless ya trae UID/GID 65532.
USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/app/server"]
