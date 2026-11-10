package render

import (
	"strings"
	"testing"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/web/view"
)

func mustRender(t *testing.T, name string, data any) string {
	t.Helper()
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Execute(name, data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return string(b)
}

// Regresión: con un único conjunto de plantillas, el bloque "content" de la última página pisaba al
// de las demás y todas las rutas mostraban el formulario de subida.
func TestCadaPaginaMuestraSuPropioContenido(t *testing.T) {
	home := mustRender(t, "home.html", view.Home{Base: view.Base{Nav: "upload"}, MaxMiB: 10})
	list := mustRender(t, "documents_list.html", view.BuildList(view.ParseListQuery("", "", "", "", ""), view.ListPayload{}))
	detail := mustRender(t, "document_detail.html", view.BuildDetail(view.PersistedDoc{DocumentID: "id", Filename: "a.pdf", SummaryStatus: "NOT_REQUESTED"}, "id"))
	text := mustRender(t, "document_text.html", view.BuildFullText(view.PersistedDoc{DocumentID: "id", ExtractedText: "x"}, "id"))
	errp := mustRender(t, "error.html", view.ErrorPage{Title: "Oops", Message: "falló", BackURL: "/", BackLabel: "Volver"})

	check := func(page, html, want, notWant string) {
		t.Helper()
		if !strings.Contains(html, want) {
			t.Errorf("%s: falta %q", page, want)
		}
		if notWant != "" && strings.Contains(html, notWant) {
			t.Errorf("%s: no debería contener %q", page, notWant)
		}
	}
	check("home", home, "Subir y procesar", "Todavía no hay documentos")
	check("home", home, "Generar resumen con IA", "")
	check("list", list, "Todavía no hay documentos", "Subir y procesar")
	check("detail", detail, "Borrar documento", "Subir y procesar")
	check("text", text, "Texto extraído", "Subir y procesar")
	check("error", errp, "falló", "Subir y procesar")
}

func TestBaseMarcaLaNavegacionActiva(t *testing.T) {
	html := mustRender(t, "home.html", view.Home{Base: view.Base{Nav: "upload"}, MaxMiB: 10})
	if !strings.Contains(html, `<a href="/" aria-current="page">Subir PDF</a>`) {
		t.Fatal("falta aria-current en Subir PDF")
	}
	if strings.Contains(html, `<a href="/documents" aria-current="page">`) {
		t.Fatal("Documentos no debería estar activo")
	}
}

func TestDetalleEscapaContenidoHostil(t *testing.T) {
	evil := `<script>alert(1)</script>`
	s := evil
	html := mustRender(t, "document_detail.html", view.BuildDetail(view.PersistedDoc{
		DocumentID: "id", Filename: evil, ExtractedText: evil, Summary: &s, SummaryStatus: "RESOLVED",
	}, "id"))
	if strings.Contains(html, evil) {
		t.Fatal("el contenido hostil se imprimió sin escapar")
	}
	if !strings.Contains(html, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatal("falta el contenido escapado")
	}
}

func TestDetalleEnEsperaRecargaSolo(t *testing.T) {
	d := view.Detail{ID: "abc", Wait: view.NewWait("abc", view.WaitDoc, 2)}
	html := mustRender(t, "document_detail.html", d)
	if !strings.Contains(html, `<meta http-equiv="refresh" content="2;url=/documents/abc?wait=doc&amp;n=3">`) {
		t.Fatalf("falta el refresh con la URL correcta:\n%s", html)
	}
	if !strings.Contains(html, "Guardando el documento") {
		t.Fatal("falta el mensaje de espera")
	}
	if strings.Contains(html, "Borrar documento") {
		t.Fatal("sin documento cargado no se ofrece borrar")
	}
}

func TestDetalleSinEsperaNoRecarga(t *testing.T) {
	html := mustRender(t, "document_detail.html", view.BuildDetail(view.PersistedDoc{DocumentID: "id", SummaryStatus: "NOT_REQUESTED"}, "id"))
	if strings.Contains(html, "http-equiv") {
		t.Fatal("no debería recargar")
	}
	if !strings.Contains(html, "Generar resumen") {
		t.Fatal("falta el botón para pedir resumen")
	}
}

func TestDetalleSinInlineParaLaCSP(t *testing.T) {
	// La CSP es default-src 'self': un atributo on*= o style= en línea queda bloqueado en silencio.
	pages := map[string]any{
		"home.html":           view.Home{MaxMiB: 10},
		"document_detail.html": view.BuildDetail(view.PersistedDoc{DocumentID: "id", SummaryStatus: "FAILED"}, "id"),
		"documents_list.html": view.BuildList(view.ParseListQuery("", "", "", "", ""), view.ListPayload{}),
	}
	for name, data := range pages {
		html := mustRender(t, name, data)
		for _, bad := range []string{" onclick=", " onsubmit=", " onload=", " style=", "<style", "hx-"} {
			if strings.Contains(html, bad) {
				t.Errorf("%s contiene %q, bloqueado por la CSP", name, bad)
			}
		}
		if strings.Contains(html, "<script>") {
			t.Errorf("%s contiene un script en línea", name)
		}
	}
}

func TestListadoConFilasYBorrados(t *testing.T) {
	deleted := view.PersistedDoc{DocumentID: "d1", Filename: "viejo.pdf", SummaryStatus: "NOT_REQUESTED"}
	now := view.PersistedDoc{DocumentID: "a1", Filename: "nuevo.pdf", SummaryStatus: "RESOLVED"}
	l := view.BuildList(view.ParseListQuery("", "", "", "", "true"), view.ListPayload{Total: 2, Items: []view.PersistedDoc{now, deleted}})
	l.Rows[1].Deleted = true
	html := mustRender(t, "documents_list.html", l)
	if !strings.Contains(html, `href="/documents/a1"`) {
		t.Error("falta el enlace al documento activo")
	}
	if strings.Contains(html, `href="/documents/d1"`) {
		t.Error("un documento borrado no se puede abrir")
	}
	if !strings.Contains(html, `action="/documents/d1/restore"`) {
		t.Error("falta restaurar en la fila borrada")
	}
}
