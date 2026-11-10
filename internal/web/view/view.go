// Package view arma los modelos que consumen las plantillas HTML. No depende de gin: se prueba sin
// levantar un servidor, y las plantillas reciben datos ya formateados (tiempos, estados, paginación)
// en lugar de decidir cosas ellas mismas.
package view

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Estados de resumen que publica el Persistidor (SummaryStatus de openapi.yaml).
const (
	StatusNotRequested = "NOT_REQUESTED"
	StatusPending      = "PENDING"
	StatusResolved     = "RESOLVED"
	StatusFailed       = "FAILED"
)

// Límites de la interfaz.
const (
	MaxUploadMiB    = 10
	PreviewRunes    = 3000
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Base son los datos comunes de toda página: qué entrada del menú se marca y los mensajes que la
// página muestra arriba (éxito y error).
type Base struct {
	Nav    string
	Notice string
	Error  string
}

// Status es un estado listo para pintar: el código crudo, su etiqueta en español y una clase CSS.
type Status struct {
	Code  string
	Label string
	Class string
}

// StatusOf traduce un estado de resumen; un documento borrado se muestra como tal.
func StatusOf(code string, deleted bool) Status {
	if deleted {
		return Status{Code: code, Label: "Borrado", Class: "bad"}
	}
	switch code {
	case StatusNotRequested:
		return Status{Code: code, Label: "Sin resumen", Class: "muted"}
	case StatusPending:
		return Status{Code: code, Label: "Resumen en proceso", Class: "warn"}
	case StatusResolved:
		return Status{Code: code, Label: "Resumen listo", Class: "ok"}
	case StatusFailed:
		return Status{Code: code, Label: "Resumen fallido", Class: "bad"}
	default:
		return Status{Code: code, Label: "Desconocido", Class: "muted"}
	}
}

// NoticeText traduce el código de un aviso de éxito que viaja en la query tras una redirección. Solo
// acepta códigos conocidos: lo que llega por la URL nunca se muestra tal cual.
func NoticeText(code string) string {
	switch code {
	case "uploaded":
		return "Documento recibido y procesado."
	case "duplicate":
		return "Ese contenido ya estaba cargado. Te mostramos el documento que ya existe."
	case "summary_requested":
		return "Resumen solicitado."
	case "restored":
		return "Documento restaurado."
	case "deleted":
		return "Documento borrado."
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// Modelo que devuelve el Persistidor (a través del proxy del Orquestador)
// ---------------------------------------------------------------------------

// PersistedDoc es el Document de openapi.yaml. Solo se leen los campos que la web muestra.
type PersistedDoc struct {
	DocumentID       string     `json:"document_id"`
	Filename         string     `json:"filename"`
	MimeType         string     `json:"mime_type"`
	Checksum         string     `json:"checksum"`
	ExtractedText    string     `json:"extracted_text"`
	Summary          *string    `json:"summary"`
	SummaryStatus    string     `json:"summary_status"`
	ExtractionTimeMS int64      `json:"extraction_time_ms"`
	SummaryTimeMS    *int64     `json:"summary_time_ms"`
	ProcessingTimeMS int64      `json:"processing_time_ms"`
	CreatedAt        time.Time  `json:"created_at"`
	DeletedAt        *time.Time `json:"deleted_at"`
}

// ListPayload es el DocumentList de openapi.yaml.
type ListPayload struct {
	Items    []PersistedDoc `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// ---------------------------------------------------------------------------
// Espera con recarga automática
// ---------------------------------------------------------------------------

// Tipos de espera. Se transportan en la query (`wait` y `n`) y la página se recarga sola mientras
// no llegue lo esperado, hasta MaxWaitAttempts intentos.
const (
	WaitDoc     = "doc"     // se acaba de subir: el Persistidor todavía no asentó el documento (404 transitorio)
	WaitSummary = "summary" // se pidió un resumen: todavía no figura como resuelto
	WaitPending = "pending" // el documento ya existe con el resumen en proceso

	MaxWaitAttempts = 30 // × WaitSeconds = ventana de 60 s del contrato (openapi.yaml, GET /documents/{id})
	WaitSeconds     = 2
)

// Wait describe si la página debe recargarse sola y hacia dónde.
type Wait struct {
	Active   bool
	TimedOut bool
	Kind     string
	N        int
	Secs     int
	URL      string
	Message  string
}

// ParseWait valida los parámetros de espera de la query. Un tipo desconocido o un contador fuera de
// rango se descartan: la URL la controla el usuario y no puede alargar la espera.
func ParseWait(kind, n string) (string, int) {
	switch kind {
	case WaitDoc, WaitSummary, WaitPending:
	default:
		return "", 0
	}
	v, err := strconv.Atoi(n)
	if err != nil || v < 0 {
		return kind, 0
	}
	if v > MaxWaitAttempts {
		v = MaxWaitAttempts
	}
	return kind, v
}

// NewWait arma la espera del intento n. Agotados los intentos deja de recargar y avisa.
func NewWait(id, kind string, n int) Wait {
	w := Wait{Kind: kind, N: n, Secs: WaitSeconds}
	if n >= MaxWaitAttempts {
		w.TimedOut = true
		w.Message = "Está tardando más de lo normal. Probá actualizar la página en unos segundos."
		return w
	}
	w.Active = true
	w.URL = "/documents/" + url.PathEscape(id) + "?wait=" + kind + "&n=" + strconv.Itoa(n+1)
	switch kind {
	case WaitDoc:
		w.Message = "Guardando el documento…"
	case WaitSummary:
		w.Message = "Guardando el resumen…"
	default:
		w.Message = "El resumen sigue en proceso…"
	}
	return w
}

// ---------------------------------------------------------------------------
// Inicio
// ---------------------------------------------------------------------------

// Home es la página de subida.
type Home struct {
	Base
	Summarize bool
	MaxMiB    int
}

// ---------------------------------------------------------------------------
// Detalle de un documento
// ---------------------------------------------------------------------------

// Detail es la página de un documento. Con Loaded en falso solo hay ID y Wait: el documento todavía
// no está disponible y la página espera.
type Detail struct {
	Base
	ID     string
	Loaded bool
	Wait   Wait

	Filename      string
	MimeType      string
	Checksum      string
	ShortChecksum string
	Status        Status

	Summary    string
	HasSummary bool

	TextPreview   string
	TextTruncated bool
	HasText       bool
	TextChars     string
	TextWords     string

	ExtractionTime string
	SummaryTime    string
	TotalTime      string

	CreatedISO  string
	CreatedText string

	CanRequestSummary bool
}

// BuildDetail convierte el documento del Persistidor en la página de detalle. id es el de la URL, ya
// validado como UUID, y se usa si el cuerpo no trae `document_id`.
func BuildDetail(p PersistedDoc, id string) Detail {
	if p.DocumentID != "" {
		id = p.DocumentID
	}
	st := StatusOf(p.SummaryStatus, p.DeletedAt != nil)

	d := Detail{
		ID:       id,
		Loaded:   true,
		Filename: displayName(p.Filename),
		MimeType: p.MimeType,
		Checksum: p.Checksum,
		Status:   st,

		ExtractionTime: FormatMS(p.ExtractionTimeMS),
		SummaryTime:    "—",
		TotalTime:      FormatMS(p.ProcessingTimeMS),
	}
	if len(p.Checksum) > 12 {
		d.ShortChecksum = p.Checksum[:12]
	} else {
		d.ShortChecksum = p.Checksum
	}
	if p.SummaryTimeMS != nil {
		d.SummaryTime = FormatMS(*p.SummaryTimeMS)
	}
	if p.Summary != nil && strings.TrimSpace(*p.Summary) != "" {
		d.Summary = *p.Summary
		d.HasSummary = st.Code == StatusResolved
	}
	if !p.CreatedAt.IsZero() {
		d.CreatedISO = p.CreatedAt.UTC().Format(time.RFC3339)
		d.CreatedText = p.CreatedAt.UTC().Format("02/01/2006 15:04") + " UTC"
	}

	d.HasText = strings.TrimSpace(p.ExtractedText) != ""
	d.TextChars = FormatInt(int64(utf8.RuneCountInString(p.ExtractedText)))
	d.TextWords = FormatInt(int64(len(strings.Fields(p.ExtractedText))))
	d.TextPreview, d.TextTruncated = Preview(p.ExtractedText, PreviewRunes)

	d.CanRequestSummary = p.DeletedAt == nil &&
		(st.Code == StatusNotRequested || st.Code == StatusFailed)
	return d
}

// FullText es la página que muestra el texto extraído completo.
type FullText struct {
	Base
	ID       string
	Filename string
	Text     string
	Chars    string
	Words    string
}

// BuildFullText arma la página del texto completo.
func BuildFullText(p PersistedDoc, id string) FullText {
	if p.DocumentID != "" {
		id = p.DocumentID
	}
	return FullText{
		ID:       id,
		Filename: displayName(p.Filename),
		Text:     p.ExtractedText,
		Chars:    FormatInt(int64(utf8.RuneCountInString(p.ExtractedText))),
		Words:    FormatInt(int64(len(strings.Fields(p.ExtractedText)))),
	}
}

// ---------------------------------------------------------------------------
// Listado
// ---------------------------------------------------------------------------

// ListQuery son los filtros ya validados del listado.
type ListQuery struct {
	Page           int
	PageSize       int
	Status         string
	Filename       string
	IncludeDeleted bool
}

// ParseListQuery valida los parámetros de la query. Todo valor fuera de rango vuelve al por defecto.
func ParseListQuery(page, pageSize, status, filename, includeDeleted string) ListQuery {
	q := ListQuery{Page: 1, PageSize: DefaultPageSize}
	if v, err := strconv.Atoi(page); err == nil && v >= 1 {
		q.Page = v
	}
	if v, err := strconv.Atoi(pageSize); err == nil && v >= 1 && v <= MaxPageSize {
		q.PageSize = v
	}
	switch status {
	case StatusNotRequested, StatusPending, StatusResolved, StatusFailed:
		q.Status = status
	}
	filename = strings.TrimSpace(filename)
	if utf8.RuneCountInString(filename) > 100 {
		filename = string([]rune(filename)[:100])
	}
	q.Filename = filename
	q.IncludeDeleted = includeDeleted == "true"
	return q
}

// PersisterQuery es la query que se reenvía al Persistidor.
func (q ListQuery) PersisterQuery() string {
	v := url.Values{}
	v.Set("page", strconv.Itoa(q.Page))
	v.Set("page_size", strconv.Itoa(q.PageSize))
	if q.Status != "" {
		v.Set("status", q.Status)
	}
	if q.Filename != "" {
		v.Set("filename", q.Filename)
	}
	if q.IncludeDeleted {
		v.Set("include_deleted", "true")
	}
	return v.Encode()
}

// WebURL es el enlace del listado a una página dada, conservando los filtros.
func (q ListQuery) WebURL(page int) string {
	v := url.Values{}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	if q.PageSize != DefaultPageSize {
		v.Set("page_size", strconv.Itoa(q.PageSize))
	}
	if q.Status != "" {
		v.Set("status", q.Status)
	}
	if q.Filename != "" {
		v.Set("filename", q.Filename)
	}
	if q.IncludeDeleted {
		v.Set("include_deleted", "true")
	}
	if enc := v.Encode(); enc != "" {
		return "/documents?" + enc
	}
	return "/documents"
}

// Option es una opción de un <select>.
type Option struct {
	Value    string
	Label    string
	Selected bool
}

// Row es una fila del listado.
type Row struct {
	ID             string
	Filename       string
	Status         Status
	Deleted        bool
	CreatedISO     string
	CreatedText    string
	ExtractionTime string
}

// List es la página del listado.
type List struct {
	Base
	Rows           []Row
	Total          string
	Page           int
	TotalPages     int
	From           int
	To             int
	HasPrev        bool
	HasNext        bool
	PrevURL        string
	NextURL        string
	Filename       string
	IncludeDeleted bool
	StatusOptions  []Option
	Filtered       bool
}

// BuildList convierte la respuesta del Persistidor en la página del listado.
func BuildList(q ListQuery, p ListPayload) List {
	l := List{
		Page:           q.Page,
		Filename:       q.Filename,
		IncludeDeleted: q.IncludeDeleted,
		Total:          FormatInt(p.Total),
		Filtered:       q.Status != "" || q.Filename != "" || q.IncludeDeleted,
		StatusOptions: []Option{
			{Value: "", Label: "Todos", Selected: q.Status == ""},
			{Value: StatusResolved, Label: StatusOf(StatusResolved, false).Label, Selected: q.Status == StatusResolved},
			{Value: StatusNotRequested, Label: StatusOf(StatusNotRequested, false).Label, Selected: q.Status == StatusNotRequested},
			{Value: StatusPending, Label: StatusOf(StatusPending, false).Label, Selected: q.Status == StatusPending},
			{Value: StatusFailed, Label: StatusOf(StatusFailed, false).Label, Selected: q.Status == StatusFailed},
		},
	}
	for _, it := range p.Items {
		row := Row{
			ID:             it.DocumentID,
			Filename:       displayName(it.Filename),
			Status:         StatusOf(it.SummaryStatus, it.DeletedAt != nil),
			Deleted:        it.DeletedAt != nil,
			ExtractionTime: FormatMS(it.ExtractionTimeMS),
		}
		if !it.CreatedAt.IsZero() {
			row.CreatedISO = it.CreatedAt.UTC().Format(time.RFC3339)
			row.CreatedText = it.CreatedAt.UTC().Format("02/01/2006 15:04") + " UTC"
		}
		l.Rows = append(l.Rows, row)
	}

	pageSize := p.PageSize
	if pageSize < 1 {
		pageSize = q.PageSize
	}
	l.TotalPages = int((p.Total + int64(pageSize) - 1) / int64(pageSize))
	if l.TotalPages < 1 {
		l.TotalPages = 1
	}
	if len(l.Rows) > 0 {
		l.From = (q.Page-1)*pageSize + 1
		l.To = l.From + len(l.Rows) - 1
	}
	l.HasPrev = q.Page > 1
	l.HasNext = q.Page < l.TotalPages
	if l.HasPrev {
		l.PrevURL = q.WebURL(q.Page - 1)
	}
	if l.HasNext {
		l.NextURL = q.WebURL(q.Page + 1)
	}
	return l
}

// ---------------------------------------------------------------------------
// Página de error
// ---------------------------------------------------------------------------

// ErrorPage es la página que se muestra cuando una acción no se pudo completar.
type ErrorPage struct {
	Base
	Status    int
	Title     string
	Message   string
	BackURL   string
	BackLabel string
}

// ---------------------------------------------------------------------------
// Formato
// ---------------------------------------------------------------------------

// FormatMS escribe una duración en milisegundos de forma legible: "850 ms", "1,2 s", "2 min 3 s".
func FormatMS(ms int64) string {
	switch {
	case ms < 0:
		return "—"
	case ms < 1000:
		return strconv.FormatInt(ms, 10) + " ms"
	case ms < 60_000:
		return strings.Replace(strconv.FormatFloat(float64(ms)/1000, 'f', 1, 64), ".", ",", 1) + " s"
	default:
		return fmt.Sprintf("%d min %d s", ms/60_000, (ms%60_000)/1000)
	}
}

// FormatInt separa los miles con punto, como se escribe en español: 12345 → "12.345".
func FormatInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

// Preview devuelve los primeros n caracteres (no bytes) de s, sin cortar un carácter por la mitad, y
// si hubo recorte.
func Preview(s string, n int) (string, bool) {
	if n <= 0 {
		return "", s != ""
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i], true
		}
		count++
	}
	return s, false
}

func displayName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Sin nombre"
	}
	return name
}
