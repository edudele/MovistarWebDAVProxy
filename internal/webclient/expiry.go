package webclient

import (
	"os"
	"time"

	"movistar-webdav-proxy/internal/logger"
)

// Aviso previo a la caducidad de la sesión web.
//
// La sesión (JSESSIONID) caduca a las 24 h del inicio de sesión aunque haya
// actividad. Solo se renueva sola si hay cookie persistente (PLC, "No cerrar
// sesión"); con un inicio de sesión por SMS no la hay y hay que volver a entrar.
// Para no enterarse por el corte del servicio, se avisa antes de que caduque.
const (
	defaultWarnBefore = 2 * time.Hour
	// Con renovación automática activa solo se avisa si, ya muy cerca de la
	// caducidad, la renovación no ha conseguido una sesión nueva.
	renewFailedWarnBefore = 30 * time.Minute
)

// warnBeforeExpiry es el margen del aviso (WEB_SESSION_WARN_BEFORE, p. ej. "3h").
func warnBeforeExpiry() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("WEB_SESSION_WARN_BEFORE")); err == nil && d > 0 {
		return d
	}
	return defaultWarnBefore
}

// shouldWarnExpiry decide si toca avisar de que la sesión va a caducar. Avisa una
// sola vez por cada caducidad (warnedFor es la caducidad ya avisada).
func shouldWarnExpiry(now, expires time.Time, remember bool, before time.Duration, warnedFor time.Time) bool {
	if expires.IsZero() || !expires.After(now) {
		return false // sin dato, o ya caducada (de eso avisa la alerta de caducidad)
	}
	if warnedFor.Equal(expires) {
		return false
	}
	left := expires.Sub(now)
	if remember {
		return left < renewFailedWarnBefore
	}
	return left < before
}

// SetExpiryWarning registra la función a la que se avisa cuando la sesión está a
// punto de caducar: caducidad, tiempo restante y si había renovación automática.
func (s *Session) SetExpiryWarning(f func(expires time.Time, left time.Duration, autoRenewal bool)) {
	s.stateMu.Lock()
	s.onExpiring = f
	s.stateMu.Unlock()
}

// reportExpiry deja constancia en el log de cuándo caduca la sesión y si se
// renueva sola, y lanza el aviso previo si toca. Se llama cada minuto.
func (s *Session) reportExpiry(now, expires time.Time, remember bool) {
	if expires.IsZero() || !s.LoggedIn() {
		return
	}

	s.stateMu.Lock()
	changed := !expires.Equal(s.loggedExpires) || remember != s.loggedRemember
	s.loggedExpires, s.loggedRemember = expires, remember
	warn := shouldWarnExpiry(now, expires, remember, warnBeforeExpiry(), s.warnedFor)
	hook := s.onExpiring
	if warn {
		s.warnedFor = expires
	}
	s.stateMu.Unlock()

	if changed {
		if remember {
			logger.Log("[WebSession] ℹ️ La sesión web caduca el %s; renovación automática ACTIVA (cookie de «recordarme» presente).", expires.Local().Format("02/01 15:04"))
		} else {
			logger.Log("[WebSession] ⚠️ La sesión web caduca el %s y NO se renovará sola (no hay cookie de «recordarme»): en el panel, /web-login/, pulsa «Cerrar sesión y volver a entrar» e inicia sesión con la casilla «recordarme» marcada.", expires.Local().Format("02/01 15:04"))
		}
	}
	if warn {
		left := expires.Sub(now).Round(time.Minute)
		logger.Log("[WebSession] ⏳ La sesión web caduca en %s (%s).", left, expires.Local().Format("02/01 15:04"))
		if hook != nil {
			hook(expires, left, remember)
		}
	}
}
