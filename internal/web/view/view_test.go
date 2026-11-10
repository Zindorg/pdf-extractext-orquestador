package view

import (
	"strings"
	"testing"
	"time"
)

func TestFormatMS(t *testing.T) {
	cases := map[int64]string{
		-1:      "—",
		0:       "0 ms",
		850:     "850 ms",
		1000:    "1,0 s",
		1234:    "1,2 s",
		59_999:  "60,0 s",
		60_000:  "1 min 0 s",
		123_000: "2 min 3 s",
	}
	for in, want := range cases {
		if got := FormatMS(in); got != want {
			t.Errorf("FormatMS(%d) = %q, quería %q", in, got, want)
		}
	}
}

func TestFormatInt(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 1000: "1.000", 12345: "12.345", 1234567: "1.234.567", -9876: "-9.876"}
	for in, want := range cases {
		if got := FormatInt(in); got != want {
			t.Errorf("FormatInt(%d) = %q, quería %q", in, got, want)
		}
	}
}

func TestPreviewCortaPorCaracteresNoPorBytes(t *testing.T) {
	got, cut := Preview("ñandú çafé", 4)
	if got != "ñand" || !cut {
		t.Fatalf("got %q cut=%v", got, cut)
	}
	if got, cut := Preview("corto", 10); got != "corto" || cut {
		t.Fatalf("got %q cut=%v", got, cut)
	}
	if got, cut := Preview("abc", 3); got != "abc" || cut {
		t.Fatalf("justo al límite no recorta: %q %v", got, cut)
	}
}

func TestParseWait(t *testing.T) {
	cases := []struct {
		kind, n  string
		wantKind string
		wantN    int
	}{
		{"doc", "3", "doc", 3},
		{"summary", "0", "summary", 0},
		{"pending", "x", "pending", 0},
		{"doc", "-4", "doc", 0},
		{"doc", "9999", "doc", MaxWaitAttempts},
		{"otra-cosa", "3", "", 0},
		{"", "", "", 0},
	}
	for _, tc := range cases {
		k, n := ParseWait(tc.kind, tc.n)
		if k != tc.wantKind || n != tc.wantN {
			t.Errorf("ParseWait(%q,%q) = (%q,%d), quería (%q,%d)", tc.kind, tc.n, k, n, tc.wantKind, tc.wantN)
		}
	}
}

func TestNewWait(t *testing.T) {
	w := NewWait("abc", WaitDoc, 2)
	if !w.Active || w.URL != "/documents/abc?wait=doc&n=3" || w.Secs != WaitSeconds {
		t.Fatalf("%+v", w)
	}
	w = NewWait("abc", WaitSummary, MaxWaitAttempts)
	if w.Active || !w.TimedOut || w.Message == "" {
		t.Fatalf("agotada: %+v", w)
	}
}

func TestParseListQuery(t *testing.T) {
	q := ParseListQuery("0", "500", "XX", "  contrato  ", "true")
	if q.Page != 1 || q.PageSize != DefaultPageSize || q.Status != "" || q.Filename != "contrato" || !q.IncludeDeleted {
		t.Fatalf("%+v", q)
	}
	q = ParseListQuery("3", "50", "RESOLVED", "", "")
	if q.Page != 3 || q.PageSize != 50 || q.Status != StatusResolved || q.IncludeDeleted {
		t.Fatalf("%+v", q)
	}
	long := strings.Repeat("é", 300)
	if got := ParseListQuery("", "", "", long, "").Filename; len([]rune(got)) != 100 {
		t.Fatalf("largo %d", len([]rune(got)))
	}
}

func TestListQueryURLs(t *testing.T) {
	q := ParseListQuery("2", "", "RESOLVED", "a b&c", "true")
	if got := q.PersisterQuery(); got != "filename=a+b%26c&include_deleted=true&page=2&page_size=20&status=RESOLVED" {
		t.Fatalf("persister: %s", got)
	}
	if got := q.WebURL(3); got != "/documents?filename=a+b%26c&include_deleted=true&page=3&status=RESOLVED" {
		t.Fatalf("web: %s", got)
	}
	if got := (ListQuery{Page: 1, PageSize: DefaultPageSize}).WebURL(1); got != "/documents" {
		t.Fatalf("sin filtros: %s", got)
	}
}

func TestBuildListPaginacion(t *testing.T) {
	deleted := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	q := ParseListQuery("2", "2", "", "", "")
	l := BuildList(q, ListPayload{
		Total: 5, Page: 2, PageSize: 2,
		Items: []PersistedDoc{
			{DocumentID: "a", Filename: "a.pdf", SummaryStatus: StatusResolved, CreatedAt: time.Date(2026, 10, 28, 15, 4, 0, 0, time.UTC)},
			{DocumentID: "b", Filename: "", SummaryStatus: StatusNotRequested, DeletedAt: &deleted},
		},
	})
	if l.TotalPages != 3 || !l.HasPrev || !l.HasNext || l.From != 3 || l.To != 4 {
		t.Fatalf("%+v", l)
	}
	if l.PrevURL != "/documents?page_size=2" || l.NextURL != "/documents?page=3&page_size=2" {
		t.Fatalf("urls: %q %q", l.PrevURL, l.NextURL)
	}
	if l.Rows[0].CreatedText != "28/10/2026 15:04 UTC" || l.Rows[0].Status.Label != "Resumen listo" {
		t.Fatalf("%+v", l.Rows[0])
	}
	if !l.Rows[1].Deleted || l.Rows[1].Status.Label != "Borrado" || l.Rows[1].Filename != "Sin nombre" {
		t.Fatalf("%+v", l.Rows[1])
	}
	if l.Filtered {
		t.Fatal("sin filtros no debe marcarse filtrado")
	}
}

func TestBuildListVacio(t *testing.T) {
	l := BuildList(ParseListQuery("", "", "", "", ""), ListPayload{})
	if l.TotalPages != 1 || l.HasNext || l.HasPrev || len(l.Rows) != 0 || l.Total != "0" {
		t.Fatalf("%+v", l)
	}
}

func TestBuildDetail(t *testing.T) {
	summary := "Resumen corto."
	st := int64(2500)
	p := PersistedDoc{
		DocumentID: "id-1", Filename: "informe.pdf", MimeType: "application/pdf",
		Checksum:      strings.Repeat("ab", 32),
		ExtractedText: "Hola mundo\ncon dos líneas",
		Summary:       &summary, SummaryStatus: StatusResolved,
		ExtractionTimeMS: 900, SummaryTimeMS: &st, ProcessingTimeMS: 3400,
		CreatedAt: time.Date(2026, 10, 28, 15, 4, 0, 0, time.UTC),
	}
	d := BuildDetail(p, "otro")
	if d.ID != "id-1" || !d.Loaded || !d.HasSummary || d.Summary != summary || d.CanRequestSummary {
		t.Fatalf("%+v", d)
	}
	if d.TextChars != "25" || d.TextWords != "5" || d.TextTruncated || !d.HasText {
		t.Fatalf("texto: %+v", d)
	}
	if d.ExtractionTime != "900 ms" || d.SummaryTime != "2,5 s" || d.TotalTime != "3,4 s" || len(d.ShortChecksum) != 12 {
		t.Fatalf("tiempos: %+v", d)
	}

	// Sin resumen pedido: se puede pedir; con resumen fallido: también.
	p.Summary, p.SummaryStatus, p.SummaryTimeMS = nil, StatusNotRequested, nil
	if d := BuildDetail(p, "x"); !d.CanRequestSummary || d.HasSummary || d.SummaryTime != "—" {
		t.Fatalf("%+v", d)
	}
	p.SummaryStatus = StatusFailed
	if d := BuildDetail(p, "x"); !d.CanRequestSummary {
		t.Fatalf("FAILED debe permitir reintentar: %+v", d)
	}
	p.SummaryStatus = StatusPending
	if d := BuildDetail(p, "x"); d.CanRequestSummary {
		t.Fatalf("PENDING no debe permitir pedir otra vez: %+v", d)
	}
}

func TestBuildDetailTextoLargoSeRecorta(t *testing.T) {
	p := PersistedDoc{DocumentID: "i", ExtractedText: strings.Repeat("palabra ", 2000), SummaryStatus: StatusNotRequested}
	d := BuildDetail(p, "i")
	if !d.TextTruncated || len([]rune(d.TextPreview)) != PreviewRunes {
		t.Fatalf("preview %d truncated=%v", len([]rune(d.TextPreview)), d.TextTruncated)
	}
	if d.TextChars != "16.000" {
		t.Fatalf("caracteres: %s", d.TextChars)
	}
}

func TestNoticeTextSoloCodigosConocidos(t *testing.T) {
	if NoticeText("uploaded") == "" {
		t.Fatal("uploaded debería tener texto")
	}
	if got := NoticeText("<script>alert(1)</script>"); got != "" {
		t.Fatalf("un código desconocido no se refleja: %q", got)
	}
}
