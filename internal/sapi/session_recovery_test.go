package sapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"movistar-webdav-proxy/internal/auth"
	"movistar-webdav-proxy/internal/config"
)

// newTestSessionClient crea un cliente apuntando a un SAPI falso y con las
// dependencias externas (apiseg, disco, sleeps) sustituidas.
func newTestSessionClient(t *testing.T, sapiURL string) (*Client, *persistRecorder) {
	t.Helper()
	c, err := NewClient(config.AccountConfig{
		Provider:        "o2",
		Phone:           "600000715",
		AccessToken:     "acc-1",
		RefreshToken:    "ref-1",
		LastRefreshDate: 1700000000000,
		ExpiresIn:       "3600",
	})
	if err != nil {
		t.Fatal(err)
	}
	c.SetBaseURLs(sapiURL, sapiURL)
	rec := &persistRecorder{}
	c.persistFn = rec.save
	c.loadFn = func(string) config.AccountConfig { return config.AccountConfig{} }
	c.sleepFn = func(time.Duration) {}
	c.sapiLoginFn = func(*http.Client, string, string, string, string, string, int64, string) auth.SAPILoginResult {
		return auth.SAPILoginResult{StatusCode: http.StatusUnauthorized, Error: errors.New("SAPI login returned HTTP 401")}
	}
	return c, rec
}

type persistRecorder struct {
	mu    sync.Mutex
	saved []config.AccountConfig
	fail  int
}

func (p *persistRecorder) save(cfg config.AccountConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fail > 0 {
		p.fail--
		return errors.New("disk full")
	}
	p.saved = append(p.saved, cfg)
	return nil
}

func (p *persistRecorder) last() (config.AccountConfig, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.saved) == 0 {
		return config.AccountConfig{}, 0
	}
	return p.saved[len(p.saved)-1], len(p.saved)
}

func storageOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"data":{"used":1,"quota":10}}`))
}

// Escenario real (log O2, 10:48:36): apiseg devuelve 401 al canjear el
// refresh_token pero SAPI sigue aceptando la sesión. Antes se daba la
// vinculación por perdida al primer intento.
func TestRefreshSession_Apiseg401ButSAPIStillAlive(t *testing.T) {
	sapi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageOK(w)
	}))
	defer sapi.Close()

	c, _ := newTestSessionClient(t, sapi.URL)
	var refreshCalls atomic.Int32
	c.refreshTokenFn = func(string, string) (*auth.OAuthTokens, error) {
		refreshCalls.Add(1)
		return nil, &auth.OAuthError{Op: "token refresh", StatusCode: http.StatusUnauthorized, Code: "invalid_client"}
	}
	expired := make(chan error, 1)
	c.OnSessionExpired = func(err error) { expired <- err }

	if !c.RefreshSession() {
		t.Fatal("RefreshSession must succeed when SAPI still accepts the session")
	}
	if c.IsSessionExpired() {
		t.Fatal("session must not be marked as expired")
	}
	if n := refreshCalls.Load(); n != 1 {
		t.Fatalf("a rejected (non-transient) refresh must not be retried in a loop, calls=%d", n)
	}
	select {
	case err := <-expired:
		t.Fatalf("no expiry alert expected, got %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRefreshSession_RevokedAndRejectedMarksExpiredOnceThenRecovers(t *testing.T) {
	var alive atomic.Bool
	sapi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !alive.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		storageOK(w)
	}))
	defer sapi.Close()

	c, _ := newTestSessionClient(t, sapi.URL)
	c.refreshTokenFn = func(string, string) (*auth.OAuthTokens, error) {
		return nil, &auth.OAuthError{Op: "token refresh", StatusCode: http.StatusBadRequest, Code: "invalid_grant"}
	}
	var expiredAlerts atomic.Int32
	recovered := make(chan time.Duration, 1)
	c.OnSessionExpired = func(error) { expiredAlerts.Add(1) }
	c.OnSessionRecovered = func(d time.Duration) { recovered <- d }

	if c.RefreshSession() {
		t.Fatal("RefreshSession must fail when grant is revoked and SAPI rejects the session")
	}
	if !c.IsSessionExpired() || c.ExpiredSince().IsZero() {
		t.Fatal("session must be marked as expired")
	}

	// Un intento de recuperación fallido no debe repetir la alerta.
	if c.TryRecoverSession() {
		t.Fatal("recovery must fail while SAPI keeps rejecting")
	}
	if !c.IsSessionExpired() {
		t.Fatal("session must remain expired after failed recovery")
	}
	time.Sleep(50 * time.Millisecond)
	if n := expiredAlerts.Load(); n != 1 {
		t.Fatalf("expiry alert must be sent once per incident, got %d", n)
	}

	// SAPI vuelve a aceptar la sesión: la recuperación automática la reactiva.
	alive.Store(true)
	if !c.TryRecoverSession() {
		t.Fatal("recovery must succeed once SAPI accepts the session again")
	}
	if c.IsSessionExpired() {
		t.Fatal("session must be active after recovery")
	}
	select {
	case <-recovered:
	case <-time.After(time.Second):
		t.Fatal("OnSessionRecovered was not called")
	}
}

// Los tokens rotados que llegan en una respuesta de error de
// /sapi/login/oauth deben conservarse y usarse para el canje.
func TestRefreshSession_KeepsTokensRotatedInFailedLogin(t *testing.T) {
	sapi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageOK(w)
	}))
	defer sapi.Close()

	c, rec := newTestSessionClient(t, sapi.URL)
	c.sapiLoginFn = func(_ *http.Client, _ string, access, _ string, _ string, _ string, _ int64, _ string) auth.SAPILoginResult {
		if access == "acc-1" {
			return auth.SAPILoginResult{
				StatusCode:     http.StatusUnauthorized,
				RefreshedOAuth: &auth.RotatedOAuth{AccessToken: "acc-2", RefreshToken: "ref-2", ExpiresIn: "3600"},
			}
		}
		return auth.SAPILoginResult{StatusCode: http.StatusOK, JSessionID: "js", LastRefreshDate: 1700000001000}
	}
	var usedRefresh string
	c.refreshTokenFn = func(refresh, _ string) (*auth.OAuthTokens, error) {
		usedRefresh = refresh
		return &auth.OAuthTokens{AccessToken: "acc-3", RefreshToken: "ref-3", ExpiresIn: 7200}, nil
	}

	if !c.RefreshSession() {
		t.Fatal("RefreshSession should succeed")
	}
	if usedRefresh != "ref-2" {
		t.Fatalf("refresh exchange must use the token rotated by SAPI, used %q", usedRefresh)
	}
	cfg := c.GetConfig()
	if cfg.RefreshToken != "ref-3" || cfg.AccessToken != "acc-3" || cfg.ExpiresIn != "7200" {
		t.Fatalf("unexpected final credentials: %+v", cfg)
	}
	if last, n := rec.last(); n == 0 || last.RefreshToken != "ref-3" {
		t.Fatalf("final credentials must be persisted, last=%+v n=%d", last, n)
	}
}

// Tras canjear el refresh_token, si /sapi/login/oauth da 401 pero la API
// acepta los tokens nuevos, la sesión es válida (la vinculación inicial ya
// no usa ese endpoint).
func TestRefreshSession_LoginEndpoint401ButAPIAcceptsNewTokens(t *testing.T) {
	sapi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "oauth ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		storageOK(w)
	}))
	defer sapi.Close()

	c, _ := newTestSessionClient(t, sapi.URL)
	c.refreshTokenFn = func(string, string) (*auth.OAuthTokens, error) {
		return &auth.OAuthTokens{AccessToken: "acc-new", RefreshToken: "ref-new"}, nil
	}
	if !c.RefreshSession() {
		t.Fatal("new tokens accepted by the API must not be discarded")
	}
	if c.IsSessionExpired() {
		t.Fatal("session must stay active")
	}
}

func TestRefreshSession_TransientErrorsAreRetried(t *testing.T) {
	sapi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer sapi.Close()

	c, _ := newTestSessionClient(t, sapi.URL)
	var calls atomic.Int32
	c.refreshTokenFn = func(string, string) (*auth.OAuthTokens, error) {
		calls.Add(1)
		return nil, &auth.OAuthError{Op: "token refresh", StatusCode: http.StatusBadGateway}
	}
	if c.RefreshSession() {
		t.Fatal("RefreshSession must fail")
	}
	if calls.Load() != 3 {
		t.Fatalf("transient errors must be retried 3 times, got %d", calls.Load())
	}
	if c.IsSessionExpired() {
		t.Fatal("a transient error must not mark the session as expired")
	}
	if !c.RefreshTransient() {
		t.Fatal("RefreshTransient must be set")
	}
}

func TestClassifyRefreshError(t *testing.T) {
	cases := []struct {
		err  error
		want refreshErrClass
	}{
		{&auth.OAuthError{StatusCode: 400, Code: "invalid_grant"}, refreshErrRevoked},
		{&auth.OAuthError{StatusCode: 401, Code: "invalid_client"}, refreshErrRejected},
		{&auth.OAuthError{StatusCode: 401}, refreshErrRejected},
		{&auth.OAuthError{StatusCode: 503}, refreshErrTransient},
		{&auth.OAuthError{StatusCode: 429}, refreshErrTransient},
		{errors.New("connection reset by peer"), refreshErrTransient},
		{errors.New("invalid_grant"), refreshErrRevoked},
	}
	for _, tc := range cases {
		if got := classifyRefreshError(tc.err); got != tc.want {
			t.Errorf("classifyRefreshError(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}

func TestPersistConfigRetriesOnFailure(t *testing.T) {
	c, rec := newTestSessionClient(t, "http://127.0.0.1:1")
	rec.fail = 1
	c.persistConfig(config.AccountConfig{RefreshToken: "ref-x"})
	if last, n := rec.last(); n != 1 || last.RefreshToken != "ref-x" {
		t.Fatalf("persistConfig must retry once after a failed write, n=%d last=%+v", n, last)
	}
}

// Una respuesta 2xx demuestra que la sesión funciona aunque estuviera
// marcada como expirada.
func TestSuccessfulResponseClearsExpiredState(t *testing.T) {
	sapi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageOK(w)
	}))
	defer sapi.Close()

	c, _ := newTestSessionClient(t, sapi.URL)
	recovered := make(chan struct{}, 1)
	c.OnSessionRecovered = func(time.Duration) { recovered <- struct{}{} }
	c.markSessionExpired(errors.New("test"))

	if _, err := c.GetStorageSpace(); err != nil {
		t.Fatalf("GetStorageSpace: %v", err)
	}
	if c.IsSessionExpired() {
		t.Fatal("a 2xx response must clear the expired state")
	}
	select {
	case <-recovered:
	case <-time.After(time.Second):
		t.Fatal("OnSessionRecovered was not called")
	}
}

// Los reintentos por 429/503 deben presentar los tokens vigentes, no los de
// la petición original, si SAPI los ha rotado entretanto.
func TestBackoffRetryUsesRotatedCredentials(t *testing.T) {
	rotated := auth.BuildOAuthHeader("acc-rot", "ref-rot", 1700000002000, "3600")
	var calls atomic.Int32
	var secondAuth atomic.Value
	sapi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Authorization", rotated)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		secondAuth.Store(r.Header.Get("Authorization"))
		storageOK(w)
	}))
	defer sapi.Close()

	c, _ := newTestSessionClient(t, sapi.URL)
	if _, err := c.GetStorageSpace(); err != nil {
		t.Fatalf("GetStorageSpace: %v", err)
	}
	got, _ := secondAuth.Load().(string)
	parsed := auth.ParseOAuthResponseHeader(got)
	if parsed == nil || parsed.AccessToken != "acc-rot" || parsed.RefreshToken != "ref-rot" {
		t.Fatalf("retry must carry rotated credentials, got %+v", parsed)
	}
}
