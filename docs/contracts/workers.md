# Contratos con los workers

> **PROPUESTA.** Los servicios de extracción y de resumen todavía no tienen repositorio. Estos contratos
> son la propuesta del Orquestador; hay que acordarlos con quienes los implementen. Los tests de contrato
> de este repo son los que los mantienen honestos.

## Contrato común

<!-- TODO: proto HTTP, sin streaming obligatorio, 2 reintentos en error de red -->

## Extractor

### `POST /extract`

<!-- TODO: request, response, y el enum de motivos de fallo -->

## Resumidor

### `POST /summarize`

<!-- TODO: request, response, y el motivo `document_too_long` -->

## Puntos a negociar

<!-- TODO: lista de lo que depende de la implementación de cada worker -->
