package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// El panel debe poder pintarse sin acceso a internet: el CSS de Tailwind va
// embebido en el binario y no se carga desde ningún CDN.
func TestLoginPageUsaCSSEmbebidoSinCDN(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	(&Server{}).renderLoginPage(rec, req, "", "")

	body := rec.Body.String()
	for _, prohibido := range []string{"cdn.tailwindcss.com", "fonts.googleapis.com", "<script src="} {
		if strings.Contains(body, prohibido) {
			t.Errorf("la página de login no debe contener %q", prohibido)
		}
	}
	if !strings.Contains(body, ".bg-blue-600") {
		t.Error("la página de login debe incluir el CSS de Tailwind embebido (falta .bg-blue-600)")
	}
	if len(tailwindCSS) == 0 {
		t.Error("tailwindCSS embebido está vacío")
	}
}

func TestPanelCSPSinCDN(t *testing.T) {
	if strings.Contains(panelCSP, "cdn.") || strings.Contains(panelCSP, "googleapis") {
		t.Errorf("panelCSP no debe permitir CDNs externos: %s", panelCSP)
	}
}

func TestIndexTemplateTieneMarcadorCSS(t *testing.T) {
	if !strings.Contains(string(indexHTML), "/*TAILWIND_CSS*/") {
		t.Error("index.html debe contener el marcador /*TAILWIND_CSS*/ para inyectar el CSS")
	}
}
