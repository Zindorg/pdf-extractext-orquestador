package handlers

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/render"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/view"
)

const testID = "11111111-2222-4333-8444-555555555555"

type env struct {
	r       *gin.Engine
	extract *extractStub
	summary *summaryStub
	proxy   *proxyStub
}

func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rend, err := render.New()
	if err != nil {
		t.Fatal(err)
	}
	e := &env{extract: &extractStub{}, summary: &summaryStub{}, proxy: &proxyStub{}}
	h := New(Deps{Extract: e.extract, Summary: e.summary, Proxy: e.proxy}, rend)
	r := gin.New()
	r.GET("/", h.Home)
	r.GET("/documents", h.List)
	r.GET("/documents/:id", h.Detail)
	r.GET("/documents/:id/text", h.FullText)
	r.GET("/documents/:id/download/original", h.DownloadOriginal)
	r.GET("/documents/:id/download/summary", h.DownloadSummary)
	r.POST("/upload", h.Upload)
	r.POST("/documents/:id/summary", h.RequestSummary)
	r.POST("/documents/:id/delete", h.Delete)
	r.POST("/documents/:id/restore", h.Restore)
	r.NoRoute(h.NotFound)
	e.r = r
	return e
}

func (e *env) do(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.r.ServeHTTP(rec, req)
	return rec
}

func (e *env) get(path string) *httptest.ResponseRecorder {
	return e.do(httptest.NewRequest(http.MethodGet, path, nil))
}

func (e *env) post(path string) *httptest.ResponseRecorder {
	return e.do(httptest.NewRequest(http.MethodPost, path, nil))
}

func (e *env) upload(filename string, content []byte, summarize bool) *httptest.ResponseRecorder {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if filename != "" {
		fw, _ := w.CreateFormFile("document", filename)
		_, _ = fw.Write(content)
	}
	if summarize {
		_ = w.WriteField("summarize", "true")
	}
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return e.do(req)
}

func notFound() error {
	return domain.NewError("document_not_found", "no existe")
}

const docJSON = `{"document_id":"` + testID + `","filename":"informe.pdf","mime_type":"application/pdf",
"checksum":"abcdef0123456789abcdef","extracted_text":"Hola mundo desde el PDF","summary":"Un saludo.",
"summary_status":"RESOLVED","extraction_time_ms":120,"summary_time_ms":900,"processing_time_ms":1100,
"created_at":"2026-10-28T15:04:00Z"}`

func okResp(body string) domain.PersisterResponse {
	return domain.PersisterResponse{StatusCode: 200, Body: []byte(body)}
}

func TestSubidaExitosaRedirigeAlDetalleEnEspera(t *testing.T) {
	e := newEnv(t)
	e.extract.result = domain.IngestResult{DocumentID: domain.DocumentID(testID), DeliveryStatus: domain.DeliveryIngested}
	rec := e.upload("../../ruta/informe.pdf", []byte("%PDF-1.4 x"), true)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("estado %d: %s", rec.Code, rec.Body)
	}
	if got, want := rec.Header().Get("Location"), "/documents/"+testID+"?notice=uploaded&wait=doc&n=0"; got != want {
		t.Fatalf("Location %q, quería %q", got, want)
	}
	if !e.extract.called || !e.extract.in.Summarize || e.extract.in.Filename != "informe.pdf" {
		t.Fatalf("entrada al caso de uso: %+v", e.extract.in)
	}
}

func TestSubidaDuplicadaVaDirectoAlDocumento(t *testing.T) {
	e := newEnv(t)
	e.extract.result = domain.IngestResult{DocumentID: domain.DocumentID(testID), DeliveryStatus: domain.DeliveryAvailable}
	rec := e.upload("a.pdf", []byte("%PDF-1.4 x"), false)
	if got, want := rec.Header().Get("Location"), "/documents/"+testID+"?notice=duplicate"; got != want {
		t.Fatalf("Location %q, quería %q", got, want)
	}
}

func TestSubidaSinArchivoVacioYGrande(t *testing.T) {
	e := newEnv(t)
	if rec := e.upload("", nil, false); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Seleccioná un archivo") {
		t.Fatalf("sin archivo: %d", rec.Code)
	}
	if rec := e.upload("a.pdf", nil, false); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "vacío") {
		t.Fatalf("vacío: %d", rec.Code)
	}
	big := bytes.Repeat([]byte("x"), (view.MaxUploadMiB<<20)+10)
	if rec := e.upload("a.pdf", big, false); rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "10 MiB") {
		t.Fatalf("grande: %d", rec.Code)
	}
	if e.extract.called {
		t.Fatal("no debió llegar al caso de uso")
	}
}

func TestSubidaConErrorDeDominioMuestraElFormularioConMensaje(t *testing.T) {
	e := newEnv(t)
	e.extract.err = domain.NewError("unsupported_media_type", "no es un PDF")
	rec := e.upload("a.pdf", []byte("no soy pdf"), true)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("estado %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Subir y procesar") {
		t.Fatal("debería volver a mostrar el formulario")
	}
	if !strings.Contains(body, `value="true" checked`) && !strings.Contains(body, "checked") {
		t.Fatal("debería conservar la casilla de resumen")
	}
}

func TestDetalleMuestraTextoYResumen(t *testing.T) {
	e := newEnv(t)
	e.proxy.resp = okResp(docJSON)
	rec := e.get("/documents/" + testID + "?notice=uploaded")
	if rec.Code != 200 {
		t.Fatalf("estado %d", rec.Code)
	}
	b := rec.Body.String()
	for _, want := range []string{"informe.pdf", "Hola mundo desde el PDF", "Un saludo.", "Documento recibido y procesado.", "/download/original", "/download/summary"} {
		if !strings.Contains(b, want) {
			t.Errorf("falta %q", want)
		}
	}
	if strings.Contains(b, "http-equiv") {
		t.Error("un documento resuelto no debe recargarse")
	}
	if e.proxy.req.Path != "/api/v1/documents/"+testID {
		t.Errorf("ruta al persistidor: %q", e.proxy.req.Path)
	}
}

func TestDetalleRecienSubidoEsperaEnVezDeFallar(t *testing.T) {
	e := newEnv(t)
	e.proxy.err = notFound()
	rec := e.get("/documents/" + testID + "?notice=uploaded&wait=doc&n=3")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `http-equiv="refresh"`) {
		t.Fatalf("debería esperar: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "n=4") {
		t.Fatal("el contador debe avanzar")
	}
}

func TestDetalleNoEncontradoEsTerminalPasadaLaVentana(t *testing.T) {
	e := newEnv(t)
	e.proxy.err = notFound()
	for _, path := range []string{
		"/documents/" + testID,                       // sin espera: 404 directo
		"/documents/" + testID + "?wait=doc&n=30",    // ventana agotada
		"/documents/" + testID + "?wait=summary&n=1", // la espera de resumen no tolera 404
	} {
		rec := e.get(path)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: estado %d", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "http-equiv") {
			t.Errorf("%s: no debe recargar", path)
		}
	}
}

func TestDetalleConResumenPendienteRecargaYAcotaElContador(t *testing.T) {
	e := newEnv(t)
	e.proxy.resp = okResp(strings.Replace(docJSON, `"summary_status":"RESOLVED"`, `"summary_status":"PENDING"`, 1))
	rec := e.get("/documents/" + testID)
	if !strings.Contains(rec.Body.String(), `http-equiv="refresh"`) {
		t.Fatal("PENDING debe recargar")
	}
	rec = e.get("/documents/" + testID + "?wait=pending&n=99999")
	if strings.Contains(rec.Body.String(), "http-equiv") {
		t.Fatal("agotado el contador no debe seguir recargando")
	}
}

func TestDetalleErrorDelPersistidorEsPaginaPropia(t *testing.T) {
	e := newEnv(t)
	e.proxy.err = http.ErrHandlerTimeout
	rec := e.get("/documents/" + testID)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "Volver al listado") {
		t.Fatalf("estado %d", rec.Code)
	}
}

func TestListadoConFilasYPaginador(t *testing.T) {
	e := newEnv(t)
	e.proxy.resp = okResp(`{"items":[` + docJSON + `],"total":45,"page":2,"page_size":20}`)
	rec := e.get("/documents?page=2&status=RESOLVED&filename=inf")
	if rec.Code != 200 {
		t.Fatalf("estado %d", rec.Code)
	}
	b := rec.Body.String()
	for _, want := range []string{"informe.pdf", `href="/documents/` + testID + `"`, "page=3"} {
		if !strings.Contains(b, want) {
			t.Errorf("falta %q", want)
		}
	}
	if q := e.proxy.req.RawQuery; !strings.Contains(q, "page=2") || !strings.Contains(q, "status=RESOLVED") || !strings.Contains(q, "filename=inf") {
		t.Errorf("query al persistidor: %q", q)
	}
}

func TestListadoNoReflejaAvisosArbitrarios(t *testing.T) {
	e := newEnv(t)
	e.proxy.resp = okResp(`{"items":[],"total":0,"page":1,"page_size":20}`)
	rec := e.get("/documents?" + url.Values{"notice": {"<b>hola</b>"}, "error": {"<i>x</i>"}}.Encode())
	if strings.Contains(rec.Body.String(), "<b>hola</b>") || strings.Contains(rec.Body.String(), "<i>x</i>") {
		t.Fatal("la query se reflejó en la página")
	}
}

func TestListadoFallaElPersistidor(t *testing.T) {
	e := newEnv(t)
	e.proxy.err = http.ErrHandlerTimeout
	rec := e.get("/documents")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "No pudimos consultar") {
		t.Fatalf("estado %d", rec.Code)
	}
}

func TestAcciones(t *testing.T) {
	e := newEnv(t)
	rec := e.post("/documents/" + testID + "/summary")
	if got := rec.Header().Get("Location"); rec.Code != 303 || got != "/documents/"+testID+"?notice=summary_requested&wait=summary&n=0" {
		t.Fatalf("resumen: %d %q", rec.Code, got)
	}
	e.summary.err = domain.NewError("summary_pending", "ya en curso")
	rec = e.post("/documents/" + testID + "/summary")
	if got := rec.Header().Get("Location"); rec.Code != 303 || got != "/documents/"+testID+"?error=summary_pending" {
		t.Fatalf("resumen con error: %d %q", rec.Code, got)
	}

	rec = e.post("/documents/" + testID + "/delete")
	if got := rec.Header().Get("Location"); rec.Code != 303 || got != "/documents?notice=deleted" {
		t.Fatalf("borrar: %d %q", rec.Code, got)
	}
	if e.proxy.req.Method != http.MethodDelete {
		t.Fatalf("método al persistidor: %s", e.proxy.req.Method)
	}

	rec = e.post("/documents/" + testID + "/restore")
	if got := rec.Header().Get("Location"); rec.Code != 303 || got != "/documents/"+testID+"?notice=restored" {
		t.Fatalf("restaurar: %d %q", rec.Code, got)
	}
	e.proxy.err = notFound()
	rec = e.post("/documents/" + testID + "/restore")
	if got := rec.Header().Get("Location"); !strings.HasPrefix(got, "/documents?include_deleted=true&error=document_not_found") {
		t.Fatalf("restaurar con error: %q", got)
	}
}

func TestDescargaCopiaCabecerasYCuerpo(t *testing.T) {
	e := newEnv(t)
	e.proxy.resp = domain.PersisterResponse{StatusCode: 200, Body: []byte("contenido"), Header: map[string][]string{
		"Content-Type":        {"text/plain; charset=utf-8"},
		"Content-Disposition": {`attachment; filename="informe.txt"`},
	}}
	rec := e.get("/documents/" + testID + "/download/original")
	if rec.Code != 200 || rec.Body.String() != "contenido" || !strings.Contains(rec.Header().Get("Content-Disposition"), "informe.txt") {
		t.Fatalf("%d %q %v", rec.Code, rec.Body, rec.Header())
	}
	if e.proxy.req.Path != "/api/v1/documents/"+testID+"/download/original" {
		t.Fatalf("ruta %q", e.proxy.req.Path)
	}
}

func TestDescargaFallidaMuestraPagina(t *testing.T) {
	e := newEnv(t)
	e.proxy.err = domain.NewError("summary_not_available", "sin resumen")
	rec := e.get("/documents/" + testID + "/download/summary")
	if rec.Code < 400 || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") || !strings.Contains(rec.Body.String(), "Resumen no disponible") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestTextoCompleto(t *testing.T) {
	e := newEnv(t)
	e.proxy.resp = okResp(docJSON)
	rec := e.get("/documents/" + testID + "/text")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Hola mundo desde el PDF") {
		t.Fatalf("%d", rec.Code)
	}
}

func TestNoRoute(t *testing.T) {
	e := newEnv(t)
	rec := e.get("/no-existe")
	if rec.Code != 404 || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("web: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	rec = e.get("/api/v1/nada")
	if rec.Code != 404 || strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("api: %d %s", rec.Code, rec.Body)
	}
}

func TestDetallePersistidorCaidoResponde503(t *testing.T) {
	e := newEnv(t)
	e.proxy.err = fmt.Errorf("%w: llamar al Persistidor: connection refused", domain.ErrPersisterUnavailable)
	if rec := e.get("/documents/" + testID); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("estado %d, quería 503", rec.Code)
	}
}
