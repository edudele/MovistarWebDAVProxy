package webclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginViewReset(t *testing.T) {
	llamado := 0
	v := &LoginView{Reset: func() error { llamado++; return nil }}
	h := v.Handler("/web-login/")

	// GET no reinicia nada.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web-login/reset", nil))
	if rec.Code != http.StatusMethodNotAllowed || llamado != 0 {
		t.Fatalf("GET: código %d, llamadas %d; quiero 405 y 0", rec.Code, llamado)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web-login/reset", nil))
	if rec.Code != http.StatusNoContent || llamado != 1 {
		t.Fatalf("POST: código %d, llamadas %d; quiero 204 y 1", rec.Code, llamado)
	}
}

func TestLoginViewResetErrorYSinFuncion(t *testing.T) {
	v := &LoginView{Reset: func() error { return errors.New("fallo") }}
	rec := httptest.NewRecorder()
	v.Handler("/web-login/").ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web-login/reset", nil))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("con error: código %d, quiero 502", rec.Code)
	}

	rec = httptest.NewRecorder()
	(&LoginView{}).Handler("/web-login/").ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/web-login/reset", nil))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("sin función: código %d, quiero 501", rec.Code)
	}
}

func TestLoginViewIncluyeBotonYAvisoDeRecordarme(t *testing.T) {
	rec := httptest.NewRecorder()
	(&LoginView{}).Handler("/web-login/").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web-login/", nil))
	body := rec.Body.String()
	for _, quiero := range []string{`id="reset"`, "recordarme"} {
		if !strings.Contains(body, quiero) {
			t.Errorf("la vista debe contener %q", quiero)
		}
	}
}

// El script que marca «recordarme» solo debe tocar la casilla del formulario de acceso
// y no volver a tratar una casilla ya manejada (para no desmarcarla en cada vuelta).
func TestRememberMeJSEsConservador(t *testing.T) {
	for _, quiero := range []string{".rememberContainer", "rmHandled", "'none'", "'ticked'"} {
		if !strings.Contains(rememberMeJS, quiero) {
			t.Errorf("rememberMeJS debe contener %q", quiero)
		}
	}
}
