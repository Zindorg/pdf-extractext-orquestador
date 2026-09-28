# Orquestación

> Contexto de la coordinación del procesamiento de documentos: el Orquestador recibe la solicitud del
> cliente, encadena extracción, resumen opcional e ingesta, y es el único punto de contacto con el resto
> de los servicios.

## Lenguaje

### Documento

- **Definición**: _TODO_
- **Evitar**: archivo, pdf, request, upload

### Identificador de documento

- **Definición**: _TODO_
- **Evitar**: correlation_id, request_id, trace_id, id

### Texto extraído

- **Definición**: _TODO_
- **Evitar**: contenido, body, output, resultado

### Resumen

- **Definición**: _TODO_
- **Evitar**: summary, resumen_ia, tl;dr

### Checksum

- **Definición**: _TODO_
- **Evitar**: hash del archivo, md5, fingerprint

### Duplicado

- **Definición**: _TODO_
- **Evitar**: repetido, conflicto, clon

### Estado de entrega

- **Definición**: _TODO_
- **Evitar**: status, estado, estado del documento

### Estado de resumen

- **Definición**: _TODO_
- **Evitar**: status, estado

### Ingesta

- **Definición**: _TODO_
- **Evitar**: guardado, persistencia, escritura

### Documento abandonado

- **Definición**: _TODO_
- **Evitar**: huérfano, zombie, colgado, resumen huérfano

### Zero-Disk

- **Definición**: _TODO_
- **Evitar**: temporal, efímero, en memoria
