package wiim

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func setupSpotifyTest(t *testing.T) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("WIIM_SPOTIFY_TOKEN", "")
	t.Setenv("SPOTIFY_TOKEN", "")
	t.Setenv("WIIM_SPOTIFY_CLIENT_ID", "")
	t.Setenv("WIIM_SPOTIFY_CLIENT_SECRET", "")
	oldAPIBaseURL := spotifyAPIBaseURL
	oldAccountsBaseURL := spotifyAccountsBaseURL
	oldHTTPClient := spotifyHTTPClient
	oldOpenBrowser := openSpotifyBrowser
	spotifyAPIBaseURL = spotifyAPIBase
	spotifyAccountsBaseURL = spotifyAccountsBase
	spotifyHTTPClient = &http.Client{Timeout: 15 * time.Second}
	openSpotifyBrowser = func(string) error { return nil }
	t.Cleanup(func() {
		spotifyAPIBaseURL = oldAPIBaseURL
		spotifyAccountsBaseURL = oldAccountsBaseURL
		spotifyHTTPClient = oldHTTPClient
		openSpotifyBrowser = oldOpenBrowser
	})
}

func TestStripSpotifyReauth(t *testing.T) {
	args, allow := stripSpotifyReauth([]string{"transfer", "dev", "--no-play", "--reauth"})
	if !allow {
		t.Fatal("expected reauth")
	}
	want := []string{"transfer", "dev", "--no-play"}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args %#v", args)
		}
	}
}

func TestNewSpotifyClientUsesEnvTokenAndTimeoutClient(t *testing.T) {
	setupSpotifyTest(t)
	t.Setenv("WIIM_SPOTIFY_TOKEN", " env-token ")
	client, err := NewSpotifyClient(false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if client.Token != "env-token" {
		t.Fatalf("token %q", client.Token)
	}
	if client.HTTPClient == nil || client.HTTPClient.Timeout != 15*time.Second {
		t.Fatalf("HTTPClient timeout = %v", client.HTTPClient)
	}
}

func TestSpotifyTokenExpiryRefreshMath(t *testing.T) {
	cases := []struct {
		name        string
		expiresAt   time.Time
		wantRefresh bool
	}{
		{name: "valid beyond skew", expiresAt: time.Now().Add(2 * time.Minute), wantRefresh: false},
		{name: "within skew", expiresAt: time.Now().Add(30 * time.Second), wantRefresh: true},
		{name: "expired", expiresAt: time.Now().Add(-1 * time.Minute), wantRefresh: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupSpotifyTest(t)
			if err := SpotifyCredentialsSetID("client-id"); err != nil {
				t.Fatal(err)
			}
			if err := SpotifyCredentialsSetSecret("client-secret"); err != nil {
				t.Fatal(err)
			}
			if err := saveSpotifyToken(spotifyTokenCache{AccessToken: "cached-access", RefreshToken: "cached-refresh", ExpiresAt: tc.expiresAt}); err != nil {
				t.Fatal(err)
			}
			refreshes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				refreshes++
				if r.URL.Path != "/api/token" {
					t.Fatalf("path %s", r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
					t.Fatalf("grant_type %q", got)
				}
				if got := r.PostForm.Get("refresh_token"); got != "cached-refresh" {
					t.Fatalf("refresh_token %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"fresh-access","token_type":"Bearer","expires_in":3600}`)
			}))
			defer server.Close()
			spotifyAccountsBaseURL = server.URL

			client, err := NewSpotifyClient(false, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			wantToken := "cached-access"
			wantRefreshes := 0
			if tc.wantRefresh {
				wantToken = "fresh-access"
				wantRefreshes = 1
			}
			if client.Token != wantToken {
				t.Fatalf("token %q, want %q", client.Token, wantToken)
			}
			if refreshes != wantRefreshes {
				t.Fatalf("refreshes %d, want %d", refreshes, wantRefreshes)
			}
			if client.HTTPClient == nil || client.HTTPClient.Timeout != 15*time.Second {
				t.Fatalf("HTTPClient timeout = %v", client.HTTPClient)
			}
		})
	}
}

func TestSpotifyRefreshFailureClearsStaleToken(t *testing.T) {
	setupSpotifyTest(t)
	if err := SpotifyCredentialsSetID("client-id"); err != nil {
		t.Fatal(err)
	}
	if err := SpotifyCredentialsSetSecret("client-secret"); err != nil {
		t.Fatal(err)
	}
	if err := saveSpotifyToken(spotifyTokenCache{AccessToken: "old", RefreshToken: "stale", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	defer server.Close()
	spotifyAccountsBaseURL = server.URL

	_, err := NewSpotifyClient(false, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "stale token cleared") {
		t.Fatalf("err %v", err)
	}
	if _, err := keyring.Get(spotifyKeyringSvc, spotifyTokenKey); err != keyring.ErrNotFound {
		t.Fatalf("token was not cleared, err %v", err)
	}
}

func TestSpotifyCredentialsSetStatusClear(t *testing.T) {
	setupSpotifyTest(t)
	if err := SpotifyCredentialsSetID("abcd1234wxyz"); err != nil {
		t.Fatal(err)
	}
	if err := SpotifyCredentialsSetSecret("secret-value"); err != nil {
		t.Fatal(err)
	}
	if err := saveSpotifyToken(spotifyTokenCache{AccessToken: "access", RefreshToken: "refresh", ExpiresAt: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	status, err := SpotifyCredentialsStatus()
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G101 -- test dummy value with placeholder stars
	checks := map[string]any{
		"configured":         true,
		"clientSecretStored": true,
		"tokenStored":        true,
		"clientID":           "abcd****wxyz",
		"tokenExpiresAt":     "2030-01-02T03:04:05Z",
	}
	for key, want := range checks {
		if got := status[key]; got != want {
			t.Fatalf("status[%s] = %#v, want %#v (full status %#v)", key, got, want, status)
		}
	}
	if _, ok := status["clientSecret"]; ok {
		t.Fatalf("status leaked clientSecret: %#v", status)
	}

	if err := SpotifyCredentialsClear(); err != nil {
		t.Fatal(err)
	}
	status, err = SpotifyCredentialsStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status["configured"] != false || status["clientSecretStored"] != false || status["tokenStored"] != false {
		t.Fatalf("status after clear %#v", status)
	}
	if _, ok := status["clientID"]; ok {
		t.Fatalf("clientID still present after clear: %#v", status)
	}
}

func TestSpotifyCallbackHandlerRejectsStateMismatch(t *testing.T) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	req := httptest.NewRequest("GET", "/login?state=wrong&code=abc", nil)
	rr := httptest.NewRecorder()

	spotifyCallbackHandler("expected", codeCh, errCh).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rr.Code)
	}
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "state mismatch") {
			t.Fatalf("err %v", err)
		}
	default:
		t.Fatal("expected error")
	}
	select {
	case code := <-codeCh:
		t.Fatalf("unexpected code %q", code)
	default:
	}
}

func TestExchangeSpotifyCodeHappyPath(t *testing.T) {
	setupSpotifyTest(t)
	var seen url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method %s", r.Method)
		}
		if r.URL.Path != "/api/token" {
			t.Fatalf("path %s", r.URL.Path)
		}
		id, secret, ok := r.BasicAuth()
		if !ok || id != "client-id" || secret != "client-secret" {
			t.Fatalf("basic auth %q %q %v", id, secret, ok)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		seen, err = url.ParseQuery(string(body))
		if err != nil {
			t.Fatal(err)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Fatalf("content-type %q", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"token_type":    "Bearer",
			"scope":         spotifyScopes,
			"expires_in":    120,
		})
	}))
	defer server.Close()
	spotifyAccountsBaseURL = server.URL

	before := time.Now()
	token, err := exchangeSpotifyCode("client-id", "client-secret", "auth-code", "http://127.0.0.1:19872/login")
	if err != nil {
		t.Fatal(err)
	}
	if seen.Get("grant_type") != "authorization_code" || seen.Get("code") != "auth-code" || seen.Get("redirect_uri") != "http://127.0.0.1:19872/login" {
		t.Fatalf("form %#v", seen)
	}
	if token.AccessToken != "access-token" || token.RefreshToken != "refresh-token" || token.TokenType != "Bearer" || token.Scope != spotifyScopes {
		t.Fatalf("token %#v", token)
	}
	if token.ExpiresAt.Before(before.Add(119*time.Second)) || token.ExpiresAt.After(time.Now().Add(121*time.Second)) {
		t.Fatalf("expiresAt %v", token.ExpiresAt)
	}
}

func TestSpotifyRequestUsesInjectedBaseURLAndClient(t *testing.T) {
	setupSpotifyTest(t)
	var gotAuth, gotContentType string
	var gotBody bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/player" {
			t.Fatalf("path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if _, err := io.Copy(&gotBody, r.Body); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	spotifyAPIBaseURL = server.URL + "/v1"
	client := &SpotifyClient{Token: "api-token", HTTPClient: &http.Client{Timeout: 15 * time.Second}}

	if err := client.Transfer("device-id", true); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer api-token" {
		t.Fatalf("auth %q", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type %q", gotContentType)
	}
	if got := gotBody.String(); !strings.Contains(got, `"device_ids":["device-id"]`) || !strings.Contains(got, `"play":true`) {
		t.Fatalf("body %s", got)
	}
}

func TestSpotifyDevicesAndPlayRequests(t *testing.T) {
	setupSpotifyTest(t)
	var seen []string
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, string(data))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	spotifyAPIBaseURL = server.URL
	client := &SpotifyClient{Token: "token", HTTPClient: server.Client()}

	if _, err := client.Devices(); err != nil {
		t.Fatal(err)
	}
	if err := client.Play("device id", "https://open.spotify.com/track/abc"); err != nil {
		t.Fatal(err)
	}
	if err := client.Play("", "spotify:album:def"); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /me/player/devices", "PUT /me/player/play?device_id=device+id", "PUT /me/player/play"}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("seen %#v", seen)
		}
	}
	if !strings.Contains(bodies[1], `"uris":["spotify:track:abc"]`) {
		t.Fatalf("track body %s", bodies[1])
	}
	if !strings.Contains(bodies[2], `"context_uri":"spotify:album:def"`) {
		t.Fatalf("context body %s", bodies[2])
	}
}

func TestSpotifyRequestResponseShapesAndErrors(t *testing.T) {
	setupSpotifyTest(t)
	cases := []struct {
		name       string
		statusCode int
		body       string
		want       any
		wantErr    string
	}{
		{name: "empty success", statusCode: http.StatusNoContent, want: map[string]any{"ok": true}},
		{name: "text success", statusCode: http.StatusOK, body: "plain text", want: "plain text"},
		{name: "api error", statusCode: http.StatusTeapot, body: "short and stout", wantErr: "HTTP 418"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			spotifyAPIBaseURL = server.URL
			client := &SpotifyClient{Token: "token", HTTPClient: server.Client()}
			got, err := client.request(http.MethodGet, "/anything", nil)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch want := tc.want.(type) {
			case string:
				if got != want {
					t.Fatalf("got %#v, want %#v", got, want)
				}
			case map[string]any:
				m, ok := got.(map[string]any)
				if !ok || m["ok"] != want["ok"] {
					t.Fatalf("got %#v, want %#v", got, want)
				}
			}
		})
	}
}

func TestSpotifyCallbackHandlerOtherOutcomes(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		code    string
		wantErr string
	}{
		{name: "success", target: "/login?state=expected&code=abc", code: "abc"},
		{name: "authorization error", target: "/login?state=expected&error=access_denied", wantErr: "authorization failed"},
		{name: "missing code", target: "/login?state=expected", wantErr: "did not include code"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			codeCh := make(chan string, 1)
			errCh := make(chan error, 1)
			rr := httptest.NewRecorder()
			spotifyCallbackHandler("expected", codeCh, errCh).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.target, nil))
			if tc.wantErr != "" {
				if rr.Code != http.StatusBadRequest {
					t.Fatalf("status %d", rr.Code)
				}
				select {
				case err := <-errCh:
					if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
						t.Fatalf("err %v", err)
					}
				default:
					t.Fatal("expected error")
				}
				return
			}
			if rr.Code != http.StatusOK {
				t.Fatalf("status %d", rr.Code)
			}
			select {
			case code := <-codeCh:
				if code != tc.code {
					t.Fatalf("code %q", code)
				}
			default:
				t.Fatal("expected code")
			}
		})
	}
}

func TestSpotifySmallHelpers(t *testing.T) {
	setupSpotifyTest(t)
	spotifyAccountsBaseURL = "https://accounts.example"
	authURL := spotifyAuthURL("client", "state", "http://127.0.0.1:19872/login")
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "accounts.example" || u.Path != "/authorize" {
		t.Fatalf("auth URL %s", authURL)
	}
	q := u.Query()
	if q.Get("client_id") != "client" || q.Get("state") != "state" || q.Get("scope") != spotifyScopes {
		t.Fatalf("query %#v", q)
	}
	addr, path, err := spotifyCallbackListen("http://127.0.0.1:19872/login")
	if err != nil || addr != "127.0.0.1:19872" || path != "/login" {
		t.Fatalf("listen %q %q %v", addr, path, err)
	}
	if _, _, err := spotifyCallbackListen("https://127.0.0.1:19872/login"); err == nil {
		t.Fatal("expected invalid redirect error")
	}
	if got := firstRedirectURI([]string{"custom"}); got != "custom" {
		t.Fatalf("first redirect %q", got)
	}
	if got := firstRedirectURI(nil); got != defaultSpotifyRedirectURI {
		t.Fatalf("default redirect %q", got)
	}
	if got := maskSecret("short"); got != "********" {
		t.Fatalf("short mask %q", got)
	}
	if state, err := randomState(); err != nil || len(state) == 0 {
		t.Fatalf("state %q %v", state, err)
	}
}

func TestLoadSpotifyCredentialsPrefersEnvironment(t *testing.T) {
	setupSpotifyTest(t)
	if err := SpotifyCredentialsSetID("keyring-id"); err != nil {
		t.Fatal(err)
	}
	if err := SpotifyCredentialsSetSecret("keyring-secret"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WIIM_SPOTIFY_CLIENT_ID", " env-id ")
	t.Setenv("WIIM_SPOTIFY_CLIENT_SECRET", " env-secret ")
	creds, err := loadSpotifyCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if creds.clientID != "env-id" || creds.clientSecret != "env-secret" {
		t.Fatalf("creds %#v", creds)
	}
}

func TestRefreshSpotifyTokenKeepsRefreshTokenWhenOmitted(t *testing.T) {
	setupSpotifyTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":60}`)
	}))
	defer server.Close()
	spotifyAccountsBaseURL = server.URL
	token, err := refreshSpotifyToken("id", "secret", "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if token.RefreshToken != "old-refresh" || token.AccessToken != "new-access" {
		t.Fatalf("token %#v", token)
	}
}

func TestSpotifyLoginHappyPathUsesLoopbackCallback(t *testing.T) {
	setupSpotifyTest(t)
	if err := SpotifyCredentialsSetID("client-id"); err != nil {
		t.Fatal(err)
	}
	if err := SpotifyCredentialsSetSecret("client-secret"); err != nil {
		t.Fatal(err)
	}
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.PostForm.Get("grant_type") != "authorization_code" || r.PostForm.Get("code") != "login-code" {
			t.Fatalf("form %#v", r.PostForm)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"login-access","refresh_token":"login-refresh","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenServer.Close()
	spotifyAccountsBaseURL = tokenServer.URL

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	redirectURI := "http://" + listener.Addr().String() + "/login"
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	authURLCh := make(chan string, 1)
	openSpotifyBrowser = func(rawURL string) error {
		authURLCh <- rawURL
		return nil
	}
	errCh := make(chan error, 1)
	go func() { errCh <- SpotifyLogin(io.Discard, redirectURI) }()

	var authURL string
	select {
	case authURL = <-authURLCh:
	case err := <-errCh:
		t.Fatalf("login returned before browser opened: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for auth URL")
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	state := u.Query().Get("state")
	if state == "" || u.Query().Get("client_id") != "client-id" {
		t.Fatalf("auth URL %s", authURL)
	}
	callbackURL := redirectURI + "?state=" + url.QueryEscape(state) + "&code=login-code"
	httpClient := &http.Client{Timeout: time.Second}
	var resp *http.Response
	for i := 0; i < 20; i++ {
		resp, err = httpClient.Get(callbackURL)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback status %d", resp.StatusCode)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for login")
	}
	token, err := loadSpotifyToken()
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "login-access" || token.RefreshToken != "login-refresh" {
		t.Fatalf("token %#v", token)
	}
}

func TestSpotifyURI(t *testing.T) {
	cases := map[string]string{
		"spotify:album:abc":                         "spotify:album:abc",
		"https://open.spotify.com/track/123?si=abc": "spotify:track:123",
		"https://open.spotify.com/playlist/pl":      "spotify:playlist:pl",
	}
	for in, want := range cases {
		if got := spotifyURI(in); got != want {
			t.Fatalf("spotifyURI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSpotifyAPIRejectsOversizedSuccessAndErrorResponses(t *testing.T) {
	setupSpotifyTest(t)
	const marker = "oversized-spotify-api-response"
	body := marker + strings.Repeat("x", int(spotifyAPIResponseLimit)-len(marker)+1)
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			spotifyAPIBaseURL = server.URL
			client := &SpotifyClient{Token: "token", HTTPClient: server.Client()}
			_, err := client.request(http.MethodGet, "/anything", nil)
			if err == nil {
				t.Fatal("expected oversized response error")
			}
			if _, ok := err.(RuntimeError); !ok {
				t.Fatalf("error type %T, want RuntimeError: %v", err, err)
			}
			if !strings.Contains(err.Error(), "response exceeds 1048576 bytes") {
				t.Fatalf("error %v", err)
			}
			if strings.Contains(err.Error(), marker) {
				t.Fatalf("error reflected oversized body: %v", err)
			}
		})
	}
}

func TestSpotifyTokenRejectsOversizedSuccessAndErrorResponses(t *testing.T) {
	setupSpotifyTest(t)
	const marker = "oversized-spotify-token-response"
	body := marker + strings.Repeat("x", int(spotifyTokenResponseLimit)-len(marker)+1)
	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			spotifyAccountsBaseURL = server.URL
			_, err := exchangeSpotifyCode("id", "secret", "code", defaultSpotifyRedirectURI)
			if err == nil {
				t.Fatal("expected oversized response error")
			}
			if _, ok := err.(RuntimeError); !ok {
				t.Fatalf("error type %T, want RuntimeError: %v", err, err)
			}
			if !strings.Contains(err.Error(), "response exceeds 65536 bytes") {
				t.Fatalf("error %v", err)
			}
			if strings.Contains(err.Error(), marker) {
				t.Fatalf("error reflected oversized body: %v", err)
			}
		})
	}
}
