package domain

// DocumentID identifica una petición de ingesta. Es un UUID v4, única y no reutilizable
// —ni siquiera tras borrar el documento— (ADR-0004).
type DocumentID string

// Checksum es el sha256 del texto extraído, en hex minúsculas (ADR-0003). Es la identidad del
// contenido, distinta de la identidad del documento.
type Checksum string

// DeliveryStatus es el estado de entrega: en qué punto está el documento respecto de haber
// sido persistido. Es un eje independiente del estado de resumen.
type DeliveryStatus string

const (
	// DeliveryIngested: publicado en el stream y pendiente de que el Persistidor lo asiente.
	DeliveryIngested DeliveryStatus = "INGESTED"
	// DeliveryAvailable: ya consultable por la API.
	DeliveryAvailable DeliveryStatus = "AVAILABLE"
)

// SummaryStatus es si el resumen fue pedido y qué le pasó. Su valor lo decide el Orquestador,
// que es el único que sabe si el cliente lo pidió. Es un eje independiente del estado de entrega.
type SummaryStatus string

const (
	// SummaryNotRequested: el cliente no lo pidió.
	SummaryNotRequested SummaryStatus = "NOT_REQUESTED"
	// SummaryPending: se pidió y se está resolviendo.
	SummaryPending SummaryStatus = "PENDING"
	// SummaryResolved: se resolvió y está publicado.
	SummaryResolved SummaryStatus = "RESOLVED"
	// SummaryFailed: se pidió y el Resumidor no respondió.
	SummaryFailed SummaryStatus = "FAILED"
)

// IncomingDocument es el documento recibido por el Orquestador, con el nombre que declaró el
// cliente (dato del cliente) y la decisión de si se pidió resumen. El archivo en sí no se guarda
// (Zero-Disk): donde existe en el proceso, existe como bytes en RAM.
type IncomingDocument struct {
	Bytes     []byte
	Filename  string
	Summarize bool
}

// Warning es un problema no fatal de la extracción. El vocabulario es abierto.
type Warning struct {
	Code    string
	Message string
}

// Extraction es el resultado de extraer el texto plano del documento. Reason es el motivo que
// declaró el Extractor: `success` cuando trae texto, o el motivo del rechazo cuando no.
type Extraction struct {
	Text     string
	TimeMS   int64
	Warnings []Warning
	Reason   ExtractionReason
}

// Summary es el resumen resuelto de un documento.
type Summary struct {
	Text   string
	TimeMS int64
}

// SummaryRequestResult es la salida del resumen a demanda. `ProcessingTimeMS` mide esta llamada
// entera —el resumen más las lecturas por proxy—, a diferencia del total de una admisión, que suma
// extracción y resumen (openapi.yaml, esquema `SummaryAccepted`).
type SummaryRequestResult struct {
	DocumentID       DocumentID
	Summary          string
	SummaryTimeMS    int64
	ProcessingTimeMS int64
}

// IngestResult es la salida del caso de uso de ingesta. Es una representación de dominio: el DTO
// HTTP (ExtractAccepted) se construye en la capa api. No lleva el tiempo total, que es la suma de
// las dos patas y se calcula donde se necesite.
type IngestResult struct {
	DocumentID       DocumentID
	Filename         string
	Checksum         Checksum
	DeliveryStatus   DeliveryStatus
	SummaryStatus    SummaryStatus
	ExtractionTimeMS int64
	SummaryTimeMS    int64
	SummaryText      string
	Warnings         []Warning
}
