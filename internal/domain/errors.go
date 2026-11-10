package domain

// ExtractionReason es un motivo del enum que publica el Extractor (workers.md). La puerta 2 lo
// consume para decidir si la extracción se admite o se rechaza con 422. `success` es el único
// motivo admisible; un motivo fuera de este enum no se traduce en 422.
type ExtractionReason string

const (
	ReasonSuccess             ExtractionReason = "success"
	ReasonEmpty               ExtractionReason = "empty"
	ReasonEncryptedPDF        ExtractionReason = "encrypted_pdf"
	ReasonPasswordProtected   ExtractionReason = "password_protected"
	ReasonCorrupt             ExtractionReason = "corrupt"
	ReasonUnsupportedFeatures ExtractionReason = "unsupported_features"
)

// Error es un error de dominio tipado. Code es el vocabulario público de la API (openapi.yaml) y la
// capa HTTP lo traduce a un status; el dominio no conoce códigos HTTP. Reason solo aplica a
// `extraction_failed` y viaja al cliente en `details.reason`.
//
// Los campos no se exportan: los errores de este paquete son valores compartidos por todo el
// proceso (se comparan con errors.Is), y un campo público dejaría que cualquiera los modificara.
type Error struct {
	code    string
	message string
	reason  ExtractionReason
}

// NewError construye un error de dominio con un código del vocabulario público.
func NewError(code, message string) *Error {
	return &Error{code: code, message: message}
}

func (e *Error) Error() string { return e.message }

// Code es el código público del error (openapi.yaml, `ErrorCode`).
func (e *Error) Code() string { return e.code }

// Message es el mensaje legible del error.
func (e *Error) Message() string { return e.message }

// Reason es el motivo del Extractor; solo lo lleva `extraction_failed`.
func (e *Error) Reason() ExtractionReason { return e.reason }

var (
	ErrDocumentTooLarge     = NewError("payload_too_large", "el documento excede el tamaño máximo permitido")
	ErrUnsupportedMediaType = NewError("unsupported_media_type", "el contenido no comienza con la firma %PDF-")
	ErrExtractorUnavailable = NewError("extractor_unavailable", "el Extractor no está disponible")
	ErrSummaryFailed        = NewError("summary_failed", "el Resumidor no respondió")
	ErrInvalidRequest       = NewError("invalid_request", "la petición no es válida")
	ErrDocumentNotFound     = NewError("document_not_found", "el documento no existe")
	ErrSummaryPending       = NewError("summary_pending", "el resumen todavía no está resuelto")
	ErrChecksumConflict     = NewError("checksum_conflict", "el checksum ya está ocupado por otro documento")

	// ErrSummaryAlreadyResolved lo produce solo el Orquestador: el Persistidor nunca lo contesta.
	// Rechaza pedir otra vez un resumen que ya está resuelto.
	ErrSummaryAlreadyResolved = NewError("summary_already_resolved", "el documento ya tiene un resumen resuelto")
	// ErrSummaryNotAvailable es un código del Persistidor (delta-persister.md): dice que el documento
	// no tiene resumen y no va a tenerlo, distinto de que todavía esté en camino.
	ErrSummaryNotAvailable = NewError("summary_not_available", "el documento no tiene resumen")
	// ErrRateLimited llega del Persistidor cuando limita al Orquestador; el borde también lo emite.
	ErrRateLimited = NewError("rate_limited", "límite de peticiones alcanzado")
	// ErrPersisterUnavailable es un fallo de infraestructura del Persistidor (caído, 5xx o sin
	// respuesta), no un problema de la petición del cliente.
	ErrPersisterUnavailable = NewError("persister_unavailable", "el Persistidor no está disponible")
)

// publicErrors indexa por su `code` los errores que el Persistidor puede contestar. Comparte el
// vocabulario público (delta-persister.md), así que su código se traduce sin re-inventar la taxonomía.
var publicErrors = map[string]*Error{
	ErrInvalidRequest.code:      ErrInvalidRequest,
	ErrDocumentNotFound.code:    ErrDocumentNotFound,
	ErrSummaryPending.code:      ErrSummaryPending,
	ErrChecksumConflict.code:    ErrChecksumConflict,
	ErrSummaryNotAvailable.code: ErrSummaryNotAvailable,
	ErrRateLimited.code:         ErrRateLimited,
}

// FromCode devuelve el error tipado de un código público, o nil si no pertenece al vocabulario. Un
// código desconocido no se mapea por aproximación: responder con un significado que nadie declaró
// es peor que no entender la respuesta.
func FromCode(code string) *Error {
	return publicErrors[code]
}

// NewExtractionFailed construye el error 422 a partir de un motivo del enum del Extractor.
func NewExtractionFailed(reason ExtractionReason) *Error {
	return &Error{
		code:    "extraction_failed",
		message: "el documento no se pudo extraer",
		reason:  reason,
	}
}
