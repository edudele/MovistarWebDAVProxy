package webclient

import "testing"

func TestSetProviderElegiElPortal(t *testing.T) {
	original := PortalURL
	defer func() { PortalURL = original }()

	casos := map[string]string{
		"movistar": "https://micloud.movistar.es",
		"o2":       "https://cloud.o2online.es",
		"O2":       "https://cloud.o2online.es",
		"":         "https://micloud.movistar.es",
	}
	for provider, want := range casos {
		SetProvider(provider)
		if PortalURL != want {
			t.Errorf("SetProvider(%q): PortalURL = %q, quiero %q", provider, PortalURL, want)
		}
	}
}
