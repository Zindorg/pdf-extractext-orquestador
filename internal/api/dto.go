package api

import (
	"io"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

// intake es lo que el borde entiende de una admisión: el documento en bytes y la decisión de pedir
// resumen. La validación de borde rechaza lo que no es multipart o trae un `summarize` no booleano
// antes de llegar al caso de uso.
type intake struct {
	Document  []byte
	Summarize bool
}

// parseIntake lee el formulario de admisión. El documento vive en RAM durante la petición y no se
// guarda (Zero-Disk).
func parseIntake(c *gin.Context) (intake, error) {
	form, err := c.MultipartForm()
	if err != nil {
		return intake{}, domain.ErrInvalidRequest
	}

	documents := form.File["document"]
	if len(documents) == 0 {
		return intake{}, domain.ErrInvalidRequest
	}
	file, err := documents[0].Open()
	if err != nil {
		return intake{}, domain.ErrInvalidRequest
	}
	defer func() { _ = file.Close() }()

	content := make([]byte, documents[0].Size)
	if _, err := io.ReadFull(file, content); err != nil {
		return intake{}, domain.ErrInvalidRequest
	}

	summarize := false
	if raw := c.PostForm("summarize"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return intake{}, domain.ErrInvalidRequest
		}
		summarize = parsed
	}

	return intake{Document: content, Summarize: summarize}, nil
}

// extractAccepted es la respuesta de una admisión (openapi.yaml, esquema `ExtractAccepted`).
// `summary_time_ms` es `null` cuando no se pidió: `0` significaría "se pidió e fue instantáneo".
type extractAccepted struct {
	DocumentID       string       `json:"document_id"`
	Filename         string       `json:"filename,omitempty"`
	Checksum         string       `json:"checksum,omitempty"`
	DeliveryStatus   string       `json:"delivery_status"`
	SummaryStatus    string       `json:"summary_status"`
	Summary          string       `json:"summary,omitempty"`
	ProcessingTimeMS int64        `json:"processing_time_ms"`
	ExtractionTimeMS int64        `json:"extraction_time_ms"`
	SummaryTimeMS    *int64       `json:"summary_time_ms"`
	Warnings         []warningDTO `json:"warnings,omitempty"`
}

type warningDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func newExtractAccepted(result domain.IngestResult) extractAccepted {
	accepted := extractAccepted{
		DocumentID:       string(result.DocumentID),
		Filename:         result.Filename,
		Checksum:         string(result.Checksum),
		DeliveryStatus:   string(result.DeliveryStatus),
		SummaryStatus:    string(result.SummaryStatus),
		Summary:          result.SummaryText,
		ProcessingTimeMS: result.ExtractionTimeMS + result.SummaryTimeMS,
		ExtractionTimeMS: result.ExtractionTimeMS,
	}
	if result.SummaryStatus != domain.SummaryNotRequested {
		summaryTime := result.SummaryTimeMS
		accepted.SummaryTimeMS = &summaryTime
	}
	for _, warning := range result.Warnings {
		accepted.Warnings = append(accepted.Warnings, warningDTO{Code: warning.Code, Message: warning.Message})
	}
	return accepted
}

// summaryAccepted es la respuesta de un resumen a demanda (openapi.yaml, esquema `SummaryAccepted`).
// `summary_status` es siempre `RESOLVED`: el resumen ya se generó y ya se publicó.
type summaryAccepted struct {
	DocumentID       string `json:"document_id"`
	SummaryStatus    string `json:"summary_status"`
	Summary          string `json:"summary,omitempty"`
	ProcessingTimeMS int64  `json:"processing_time_ms"`
}

func newSummaryAccepted(result domain.SummaryRequestResult) summaryAccepted {
	return summaryAccepted{
		DocumentID:       string(result.DocumentID),
		SummaryStatus:    string(domain.SummaryResolved),
		Summary:          result.Summary,
		ProcessingTimeMS: result.ProcessingTimeMS,
	}
}
