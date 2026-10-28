# syntax=docker/dockerfile:1

ARG GO_VERSION=1.27

# ---------- Build ----------
FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY cmd ./cmd
COPY internal ./internal
COPY web ./web

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- Runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
LABEL org.opencontainers.image.title="pdf-extractext-orquestador" \
      org.opencontainers.image.description="Punto de entrada público del sistema PDF-Extractext" \
      org.opencontainers.image.source="https://github.com/Zindorg/pdf-extractext-orquestador" \
      org.opencontainers.image.version="${VERSION}"

WORKDIR /app

COPY --from=build --chmod=0555 /out/server /app/server

USER 65532:65532

EXPOSE 8080
ENTRYPOINT ["/app/server"]