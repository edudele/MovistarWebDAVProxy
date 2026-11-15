package webclient

import (
	"strings"
	"testing"

	"github.com/chromedp/cdproto/emulation"
)

const headlessUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/156.0.7500.12 Safari/537.36"

func TestCleanUserAgentQuitaHeadless(t *testing.T) {
	got := cleanUserAgent(headlessUA)
	want := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.7500.12 Safari/537.36"
	if got != want {
		t.Fatalf("cleanUserAgent = %q, quiero %q", got, want)
	}
	if again := cleanUserAgent(got); again != got {
		t.Fatalf("no es idempotente: %q", again)
	}
}

func TestUserAgentOverrideSoloSiEsHeadless(t *testing.T) {
	normal := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.7500.12 Safari/537.36"
	if _, ok := userAgentOverride(normal); ok {
		t.Fatal("un User-Agent sin headless no debe sustituirse")
	}

	p, ok := userAgentOverride(headlessUA)
	if !ok {
		t.Fatal("un User-Agent con headless debe sustituirse")
	}
	if strings.Contains(p.UserAgent, "Headless") {
		t.Fatalf("el User-Agent aplicado aún contiene Headless: %q", p.UserAgent)
	}
	md := p.UserAgentMetadata
	if md == nil {
		t.Fatal("faltan los metadatos de pistas de cliente")
	}
	for _, b := range append(append([]string{}, brandNames(md.Brands)...), brandNames(md.FullVersionList)...) {
		if strings.Contains(b, "Headless") {
			t.Errorf("la marca %q delata el modo headless", b)
		}
	}
	if len(md.Brands) == 0 || md.Brands[0].Version != "156" {
		t.Errorf("versión mayor de las marcas = %+v, quiero 156", md.Brands)
	}
	if len(md.FullVersionList) == 0 || md.FullVersionList[0].Version != "156.0.7500.12" {
		t.Errorf("versión completa = %+v, quiero 156.0.7500.12", md.FullVersionList)
	}
	if md.Platform != "Linux" || md.Mobile {
		t.Errorf("plataforma inesperada: %+v", md)
	}
}

func brandNames(list []*emulation.UserAgentBrandVersion) []string {
	names := make([]string, 0, len(list))
	for _, b := range list {
		names = append(names, b.Brand)
	}
	return names
}
