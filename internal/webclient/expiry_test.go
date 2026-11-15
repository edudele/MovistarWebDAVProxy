package webclient

import (
	"testing"
	"time"
)

func TestShouldWarnExpiry(t *testing.T) {
	now := time.Date(2026, 11, 16, 20, 0, 0, 0, time.UTC)
	before := 2 * time.Hour
	in := func(d time.Duration) time.Time { return now.Add(d) }

	casos := []struct {
		nombre    string
		expires   time.Time
		remember  bool
		warnedFor time.Time
		want      bool
	}{
		{"sin renovación, a 3h: aún no", in(3 * time.Hour), false, time.Time{}, false},
		{"sin renovación, a 1h59: avisa", in(119 * time.Minute), false, time.Time{}, true},
		{"sin renovación, a 10min: avisa", in(10 * time.Minute), false, time.Time{}, true},
		{"ya avisada esa caducidad: no repite", in(30 * time.Minute), false, in(30 * time.Minute), false},
		{"caducidad nueva tras volver a entrar: avisa de nuevo", in(90 * time.Minute), false, in(-time.Hour), true},
		{"con renovación, a 1h: no avisa (se renovará sola)", in(time.Hour), true, time.Time{}, false},
		{"con renovación, a 29min: la renovación ha fallado, avisa", in(29 * time.Minute), true, time.Time{}, true},
		{"ya caducada: de eso avisa otra alerta", in(-time.Minute), false, time.Time{}, false},
		{"sin dato de caducidad", time.Time{}, false, time.Time{}, false},
	}
	for _, c := range casos {
		if got := shouldWarnExpiry(now, c.expires, c.remember, before, c.warnedFor); got != c.want {
			t.Errorf("%s: shouldWarnExpiry = %v, quiero %v", c.nombre, got, c.want)
		}
	}
}

func TestWarnBeforeExpiryPorDefectoYEntorno(t *testing.T) {
	t.Setenv("WEB_SESSION_WARN_BEFORE", "")
	if got := warnBeforeExpiry(); got != 2*time.Hour {
		t.Errorf("por defecto = %v, quiero 2h", got)
	}
	t.Setenv("WEB_SESSION_WARN_BEFORE", "3h30m")
	if got := warnBeforeExpiry(); got != 3*time.Hour+30*time.Minute {
		t.Errorf("con entorno = %v, quiero 3h30m", got)
	}
	t.Setenv("WEB_SESSION_WARN_BEFORE", "basura")
	if got := warnBeforeExpiry(); got != 2*time.Hour {
		t.Errorf("valor inválido = %v, quiero el de por defecto", got)
	}
}

// reportExpiry avisa una sola vez por caducidad y no avisa si la sesión no está iniciada.
func TestReportExpiryAvisaUnaVez(t *testing.T) {
	t.Setenv("WEB_SESSION_WARN_BEFORE", "2h")
	s := &Session{loggedIn: true}
	var avisos []time.Duration
	var autoRenov []bool
	s.SetExpiryWarning(func(_ time.Time, left time.Duration, auto bool) {
		avisos = append(avisos, left)
		autoRenov = append(autoRenov, auto)
	})

	now := time.Date(2026, 11, 16, 21, 0, 0, 0, time.UTC)
	expires := now.Add(90 * time.Minute)

	s.reportExpiry(now, expires, false)
	s.reportExpiry(now.Add(time.Minute), expires, false)
	s.reportExpiry(now.Add(2*time.Minute), expires, false)
	if len(avisos) != 1 {
		t.Fatalf("avisos = %d, quiero 1 (uno por caducidad)", len(avisos))
	}
	if autoRenov[0] {
		t.Error("sin cookie persistente no debe indicar renovación automática")
	}

	// Tras volver a iniciar sesión hay otra caducidad: vuelve a poder avisar.
	next := now.Add(24*time.Hour + 90*time.Minute - 2*time.Hour)
	s.reportExpiry(next.Add(-3*time.Hour), next, false)
	if len(avisos) != 1 {
		t.Fatalf("lejos de la nueva caducidad no debe avisar: %d avisos", len(avisos))
	}
	s.reportExpiry(next.Add(-100*time.Minute), next, false)
	if len(avisos) != 2 {
		t.Fatalf("con la nueva caducidad cerca debe avisar otra vez: %d avisos", len(avisos))
	}

	// Sin sesión iniciada no se avisa.
	s2 := &Session{}
	n := 0
	s2.SetExpiryWarning(func(time.Time, time.Duration, bool) { n++ })
	s2.reportExpiry(now, expires, false)
	if n != 0 {
		t.Errorf("sin sesión iniciada no debe avisar, avisos = %d", n)
	}
}
