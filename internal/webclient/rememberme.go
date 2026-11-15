package webclient

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"movistar-webdav-proxy/internal/logger"
)

// La sesión de la web caduca a las 24 h aunque haya actividad. Solo se renueva sola
// si el inicio de sesión dejó la cookie persistente (PLC), y la web solo la crea si
// en el formulario de acceso está marcada la casilla «recordarme»
// (`rememberme=true`, también en el acceso con código por SMS). Esa casilla no viene
// marcada de serie, así que la marca el propio proxy cuando aparece, para que el
// usuario no tenga que acordarse de hacerlo en cada inicio de sesión.
//
// Devuelve: none (no hay casilla), handled (ya tratada), already (ya estaba marcada),
// ticked (marcada ahora) o failed (no se pudo marcar).
const rememberMeJS = `(() => {
  const c = document.querySelector('.rememberContainer');
  if (!c) return 'none';
  const box = c.querySelector('input[type="checkbox"]') || c.querySelector('[role="checkbox"]');
  if (!box) return 'none';
  if (box.dataset.rmHandled) return 'handled';
  box.dataset.rmHandled = '1';
  const isOn = () => box.matches('input') ? box.checked : box.getAttribute('aria-checked') === 'true';
  if (isOn()) return 'already';
  box.click();
  if (!isOn()) { const l = c.querySelector('label'); if (l) l.click(); }
  return isOn() ? 'ticked' : 'failed';
})()`

// tickRememberMe marca «recordarme» en la pantalla de acceso si aparece. Se llama
// mientras no hay sesión iniciada.
func (s *Session) tickRememberMe() {
	tab, err := s.tab()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(tab, 5*time.Second)
	defer cancel()
	res, err := chromedp.Run(ctx, chromedp.Evaluate[string](rememberMeJS))
	if err != nil {
		return
	}
	switch res {
	case "ticked":
		logger.Log("[WebSession] ☑️ Marcada la casilla «recordarme» del acceso: la sesión podrá renovarse sola.")
	case "already":
		logger.Log("[WebSession] ☑️ La casilla «recordarme» del acceso ya estaba marcada.")
	case "failed":
		logger.Log("[WebSession] ⚠️ No se pudo marcar la casilla «recordarme»: márcala a mano antes de iniciar sesión.")
	}
}

// ResetLogin descarta la sesión del navegador y vuelve a la pantalla de acceso,
// para poder iniciar sesión de nuevo (p. ej. para que quede la cookie de «recordarme»
// si la sesión actual se creó sin ella). Mientras dura, ninguna otra llamada usa la
// pestaña ni las cookies.
func (s *Session) ResetLogin() error {
	s.renewMu.Lock()
	defer s.renewMu.Unlock()

	tab, err := s.tab()
	if err != nil {
		return err
	}
	if _, err := chromedp.Call(tab, network.ClearBrowserCookies, cdp.Empty{}); err != nil {
		return fmt.Errorf("borrar cookies: %w", err)
	}
	if err := chromedp.Do(tab, chromedp.Navigate(PortalURL+"/")); err != nil {
		return fmt.Errorf("recargar la web: %w", err)
	}
	s.setLoggedIn(false)
	s.stateMu.Lock()
	s.expires, s.remember = time.Time{}, false
	s.warnedFor, s.loggedExpires, s.loggedRemember = time.Time{}, time.Time{}, false
	s.stateMu.Unlock()
	logger.Log("[WebSession] 🔄 Sesión web reiniciada: inicia sesión de nuevo desde el panel (/web-login/).")
	return nil
}
