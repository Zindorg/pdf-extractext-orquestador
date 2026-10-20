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

// Error es un error de dominio tipado. `Code` es el vocabulario de la API (openapi.yaml) y la capa
// api lo mapea a un status HTTP; el dominio no conoce códigos HTTP. `Reason` solo aplica a
// `extraction_failed` y viaja al cliente en `details.reason`.
type Error struct {
	Code    string
	Message string
	Reason  ExtractionReason
}

func (e *Error) Error() string { return e.Message }

var (
	ErrDocumentTooLarge     = &Error{Code: "payload_too_large", Message: "el documento excede el tamaño máximo permitido"}
	ErrUnsupportedMediaType = &Error{Code: "unsupported_media_type", Message: "el contenido no comienza con la firma %PDF-"}
	ErrExtractorUnavailable = &Error{Code: "extractor_unavailable", Message: "el Extractor no está disponible"}
	ErrSummaryFailed        = &Error{Code: "summary_failed", Message: "el Resumidor no respondió"}
	ErrInvalidRequest       = &Error{Code: "invalid_request", Message: "la petición no es válida"}
	ErrDocumentNotFound     = &Error{Code: "document_not_found", Message: "el documento no existe"}
	ErrSummaryPending       = &Error{Code: "summary_pending", Message: "el resumen todavía no está resuelto"}
	ErrChecksumConflict     = &Error{Code: "checksum_conflict", Message: "el checksum ya está ocupado por otro documento"}
)

// publicErrors indexa los errores por su `code` público (openapi.yaml). El Persistidor comparte este
// vocabulario (delta-persister.md), así que su código se traduce sin re-inventar la taxonomía.
var publicErrors = map[string]*Error{
	ErrInvalidRequest.Code:   ErrInvalidRequest,
	ErrDocumentNotFound.Code: ErrDocumentNotFound,
	ErrSummaryPending.Code:   ErrSummaryPending,
	ErrChecksumConflict.Code: ErrChecksumConflict,
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
		Code:    "extraction_failed",
		Message: "el documento no se pudo extraer",
		Reason:  reason,
	}
}
