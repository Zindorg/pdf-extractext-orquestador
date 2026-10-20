# syntax=docker/dockerfile:1

# Un solo Dockerfile para los tres mocks, elegido por `ARG CMD`. Son binarios de prueba (ver
# compose.mocks.yml): no se despliegan y no comparten código con el servicio. Van con una imagen
# distroless igual que el servicio, para que el smoke corra en el mismo entorno que producción.
FROM golang:1.27 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY mocks ./mocks

# CMD es `extractor`, `summarizer` o `persister`; determina qué main se compila.
ARG CMD
RUN test -n "$CMD" && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mock ./mocks/${CMD}


FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="pdf-extractext-orquestador-mocks" \
      org.opencontainers.image.description="Mocks de workers para el smoke local del Orquestador" \
      org.opencontainers.image.source="https://github.com/Zindorg/pdf-extractext-orquestador"

WORKDIR /app
COPY --from=build /out/mock /app/mock

USER nonroot:nonroot

ENTRYPOINT ["/app/mock"]
