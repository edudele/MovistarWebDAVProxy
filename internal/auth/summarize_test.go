package auth

import (
	"strings"
	"testing"
)

// El resumen que se escribe en el log al diagnosticar un 401 no debe filtrar
// tokens, pero sí conservar el motivo del error.
func TestSummarizeSAPIBodyRedactaTokens(t *testing.T) {
	body := []byte(`{"error":"DEVICE_NOT_ALLOWED","access_token":"abcd1234secret","refresh_token":"r3fr3shsecret"}`)
	out := SummarizeSAPIBody(body)

	for _, secreto := range []string{"abcd1234secret", "r3fr3shsecret"} {
		if strings.Contains(out, secreto) {
			t.Errorf("el resumen no debe contener %q: %s", secreto, out)
		}
	}
	if !strings.Contains(out, "DEVICE_NOT_ALLOWED") {
		t.Errorf("el resumen debe conservar el motivo del error: %s", out)
	}
}
