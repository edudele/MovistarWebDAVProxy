package webclient

import (
	"regexp"
	"strings"

	"github.com/chromedp/cdproto/emulation"
)

// Chromium en modo headless se identifica como "HeadlessChrome/…". La web de O2
// Cloud (cloud.o2online.es) con ese User-Agent no redirige al login y se queda en
// el spinner de carga; con el de un Chromium normal sí. Por eso se presenta el
// navegador sin el prefijo "Headless", igual que lo haría el mismo Chromium con
// ventana. El resto del User-Agent (versión, sistema) no se modifica.
const headlessMarker = "HeadlessChrome"

var chromeVersionRe = regexp.MustCompile(`Chrome/(\d+)\.(\d+\.\d+\.\d+)`)

// cleanUserAgent devuelve ua sin el marcador de modo headless. Si no lo
// contiene, lo devuelve igual.
func cleanUserAgent(ua string) string {
	return strings.Replace(ua, headlessMarker, "Chrome", 1)
}

// userAgentOverride construye la sustitución de User-Agent para CDP, o false si
// no hace falta (el User-Agent ya no es de headless). Incluye los metadatos de
// pistas de cliente (Sec-CH-UA y navigator.userAgentData): sin ellos Chromium
// seguiría anunciando la marca de headless en esas cabeceras.
func userAgentOverride(natural string) (emulation.SetUserAgentOverrideParams, bool) {
	clean := cleanUserAgent(natural)
	if clean == natural {
		return emulation.SetUserAgentOverrideParams{}, false
	}
	params := emulation.SetUserAgentOverrideParams{UserAgent: clean}
	if m := chromeVersionRe.FindStringSubmatch(clean); m != nil {
		major, full := m[1], m[1]+"."+m[2]
		params.UserAgentMetadata = &emulation.UserAgentMetadata{
			Brands: []*emulation.UserAgentBrandVersion{
				{Brand: "Chromium", Version: major},
				{Brand: "Not.A/Brand", Version: "99"},
			},
			FullVersionList: []*emulation.UserAgentBrandVersion{
				{Brand: "Chromium", Version: full},
				{Brand: "Not.A/Brand", Version: "99.0.0.0"},
			},
			Platform:        "Linux",
			PlatformVersion: "",
			Architecture:    "x86",
			Model:           "",
			Mobile:          false,
		}
	}
	return params, true
}
