package wiim

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const (
	spotifyAPIBase      = "https://api.spotify.com/v1"
	spotifyAccountsBase = "https://accounts.spotify.com"
	spotifyScopes       = "user-read-playback-state user-modify-playback-state"
	spotifyKeyringSvc   = "wiim-cli"
	spotifyClientIDKey  = "spotify-client-id"
	spotifyClientSecKey = "spotify-client-secret"
	spotifyTokenKey     = "spotify-token"
)

var (
	spotifyAPIBaseURL      = spotifyAPIBase
	spotifyAccountsBaseURL = spotifyAccountsBase
	spotifyHTTPClient      = &http.Client{Timeout: 15 * time.Second}
	openSpotifyBrowser     = openBrowser
)

// SpotifyClient provides access to the Spotify Web API for device discovery,
// playback transfer, and track playback.
type SpotifyClient struct {
	Token      string
	HTTPClient *http.Client
}

type spotifyTokenCache struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type spotifyTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
}

// NewSpotifyClient creates an authenticated SpotifyClient. It first tries the
// WIIM_SPOTIFY_TOKEN or SPOTIFY_TOKEN environment variable, then the OS keychain,
// and refreshes the token if it is near expiration. If allowReauth is true and
// no valid token is available, it launches the OAuth login flow.
func NewSpotifyClient(allowReauth bool, stdout io.Writer, redirectURI ...string) (*SpotifyClient, error) {
	token := strings.TrimSpace(os.Getenv("WIIM_SPOTIFY_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("SPOTIFY_TOKEN"))
	}
	if token != "" {
		return &SpotifyClient{Token: token, HTTPClient: spotifyHTTPClient}, nil
	}
	cached, err := loadSpotifyToken()
	if err != nil {
		if allowReauth {
			if err := SpotifyLogin(stdout, firstRedirectURI(redirectURI)); err != nil {
				return nil, err
			}
			cached, err = loadSpotifyToken()
		}
		if err != nil {
			return nil, usagef("Spotify commands require WIIM_SPOTIFY_TOKEN/SPOTIFY_TOKEN or `wiim spotify login`; use `--reauth` to launch login automatically")
		}
	}
	if time.Now().After(cached.ExpiresAt.Add(-60 * time.Second)) {
		creds, err := loadSpotifyCredentials()
		if err != nil {
			return nil, err
		}
		cached, err = refreshSpotifyToken(creds.clientID, creds.clientSecret, cached.RefreshToken)
		if err != nil {
			_ = SpotifyLogout()
			if allowReauth {
				fmt.Fprintln(stdout, "Spotify token refresh failed; reauthorizing...")
				if loginErr := SpotifyLogin(stdout, firstRedirectURI(redirectURI)); loginErr != nil {
					return nil, loginErr
				}
				cached, err = loadSpotifyToken()
			}
			if err != nil {
				return nil, runtimef("Spotify token refresh failed; stale token cleared. Run `wiim spotify login` or retry with `--reauth`")
			}
		} else if err := saveSpotifyToken(cached); err != nil {
			return nil, err
		}
	}
	return &SpotifyClient{Token: cached.AccessToken, HTTPClient: spotifyHTTPClient}, nil
}

// Devices lists the user's available Spotify Connect devices.
func (s *SpotifyClient) Devices() (any, error) {
	return s.request("GET", "/me/player/devices", nil)
}

// Transfer transfers Spotify playback to the specified device. If play is true
// playback starts immediately on the target device.
func (s *SpotifyClient) Transfer(deviceID string, play bool) error {
	body := map[string]any{"device_ids": []string{deviceID}, "play": play}
	_, err := s.request("PUT", "/me/player", body)
	return err
}

// Play starts or resumes Spotify playback. If uri is a track it is queued via
// uris; otherwise it is treated as a context (album/playlist). An optional
// deviceID targets a specific device.
func (s *SpotifyClient) Play(deviceID, uri string) error {
	path := "/me/player/play"
	if deviceID != "" {
		path += "?device_id=" + url.QueryEscape(deviceID)
	}
	body := map[string]any{}
	if uri != "" {
		if strings.HasPrefix(uri, "spotify:track:") || strings.Contains(uri, "/track/") {
			body["uris"] = []string{spotifyURI(uri)}
		} else {
			body["context_uri"] = spotifyURI(uri)
		}
	}
	_, err := s.request("PUT", path, body)
	return err
}

func (s *SpotifyClient) request(method, path string, body any) (any, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, spotifyAPIBaseURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, runtimef("Spotify API request failed: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, runtimef("could not read Spotify API response: %v", err)
	}
	text := strings.TrimSpace(string(data))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, runtimef("Spotify API returned HTTP %d: %s", resp.StatusCode, responseSnippet(text))
	}
	if text == "" {
		return map[string]any{"ok": true}, nil
	}
	var value any
	if err := json.Unmarshal(data, &value); err == nil {
		return value, nil
	}
	return text, nil
}

// SpotifyCredentialsSet prompts the user for a Spotify client ID and secret and
// stores them in the OS keychain.
func SpotifyCredentialsSet(stdin io.Reader, stdout io.Writer) error {
	reader := bufio.NewReader(stdin)
	existingID, _ := keyring.Get(spotifyKeyringSvc, spotifyClientIDKey)
	if existingID == "" {
		fmt.Fprint(stdout, "Spotify client ID: ")
	} else {
		fmt.Fprintf(stdout, "Spotify client ID [%s, Enter to keep]: ", maskSecret(existingID))
	}
	clientID, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = existingID
	}
	if clientID == "" {
		return usagef("client ID is required")
	}
	if err := SpotifyCredentialsSetID(clientID); err != nil {
		return err
	}
	if err := SpotifyCredentialsSetSecretPrompt(stdout); err != nil {
		return err
	}
	return nil
}

// SpotifyCredentialsSetID stores a Spotify client ID in the OS keychain.
func SpotifyCredentialsSetID(clientID string) error {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return usagef("client ID is required")
	}
	if err := keyring.Set(spotifyKeyringSvc, spotifyClientIDKey, clientID); err != nil {
		return runtimef("could not store client ID in OS keychain: %v", err)
	}
	return nil
}

// SpotifyCredentialsSetSecret stores a Spotify client secret in the OS keychain.
func SpotifyCredentialsSetSecret(clientSecret string) error {
	clientSecret = strings.TrimSpace(clientSecret)
	if clientSecret == "" {
		return usagef("client secret is required")
	}
	if err := keyring.Set(spotifyKeyringSvc, spotifyClientSecKey, clientSecret); err != nil {
		return runtimef("could not store client secret in OS keychain: %v", err)
	}
	return nil
}

// SpotifyCredentialsSetSecretPrompt reads a Spotify client secret from the
// terminal with input echoing disabled and stores it in the OS keychain.
func SpotifyCredentialsSetSecretPrompt(stdout io.Writer) error {
	fmt.Fprint(stdout, "Spotify client secret: ")
	secretBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(stdout)
	if err != nil {
		return runtimef("could not read hidden client secret input: %v", err)
	}
	return SpotifyCredentialsSetSecret(string(secretBytes))
}

// SpotifyCredentialsImportClipboard reads a client ID or secret from the system
// clipboard and stores it in the OS keychain. Kind must be "id" or "secret".
func SpotifyCredentialsImportClipboard(kind string) error {
	value := clipboardText()
	if value == "" {
		return usagef("clipboard is empty or unavailable")
	}
	switch kind {
	case "id", "client-id":
		return SpotifyCredentialsSetID(value)
	case "secret", "client-secret":
		return SpotifyCredentialsSetSecret(value)
	default:
		return usagef("import-clipboard requires id or secret")
	}
}

// SpotifyCredentialsClear removes all Spotify credentials (ID, secret, and token)
// from the OS keychain.
func SpotifyCredentialsClear() error {
	for _, key := range []string{spotifyClientIDKey, spotifyClientSecKey, spotifyTokenKey} {
		if err := keyring.Delete(spotifyKeyringSvc, key); err != nil && err != keyring.ErrNotFound {
			return runtimef("could not delete %s from OS keychain: %v", key, err)
		}
	}
	return nil
}

// SpotifyLogout removes the stored Spotify access/refresh token from the OS keychain.
func SpotifyLogout() error {
	if err := keyring.Delete(spotifyKeyringSvc, spotifyTokenKey); err != nil && err != keyring.ErrNotFound {
		return runtimef("could not delete Spotify token from OS keychain: %v", err)
	}
	return nil
}

// SpotifyCredentialsStatus returns a map indicating whether client credentials
// and a token are stored, the masked client ID, and the token expiry time.
func SpotifyCredentialsStatus() (map[string]any, error) {
	clientID, idErr := keyring.Get(spotifyKeyringSvc, spotifyClientIDKey)
	_, secretErr := keyring.Get(spotifyKeyringSvc, spotifyClientSecKey)
	token, tokenErr := loadSpotifyToken()
	status := map[string]any{
		"configured":         idErr == nil && secretErr == nil,
		"clientSecretStored": secretErr == nil,
		"tokenStored":        tokenErr == nil,
	}
	if idErr == nil {
		status["clientID"] = maskSecret(clientID)
	}
	if tokenErr == nil {
		status["tokenExpiresAt"] = token.ExpiresAt.Format(time.RFC3339)
	}
	return status, nil
}

func spotifyCallbackHandler(state string, codeCh chan<- string, errCh chan<- error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("state"); got != state {
			errCh <- runtimef("Spotify login state mismatch")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			errCh <- runtimef("Spotify authorization failed: %s", e)
			http.Error(w, e, http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- runtimef("Spotify callback did not include code")
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "Spotify login complete. You can close this tab.")
		codeCh <- code
	}
}

// SpotifyLogin starts the OAuth authorization code flow: opens the browser,
// starts a loopback HTTP server for the callback, exchanges the code for tokens,
// and stores the result in the OS keychain. The flow times out after 3 minutes.
func SpotifyLogin(stdout io.Writer, redirectURI string) error {
	creds, err := loadSpotifyCredentials()
	if err != nil {
		return err
	}
	state, err := randomState()
	if err != nil {
		return err
	}
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	listenAddr, callbackPath, err := spotifyCallbackListen(redirectURI)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	server := &http.Server{Addr: listenAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	mux.HandleFunc(callbackPath, spotifyCallbackHandler(state, codeCh, errCh))
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer server.Close()
	authURL := spotifyAuthURL(creds.clientID, state, redirectURI)
	fmt.Fprintf(stdout, "Opening Spotify authorization URL:\n%s\n", authURL)
	_ = openSpotifyBrowser(authURL)
	select {
	case code := <-codeCh:
		token, err := exchangeSpotifyCode(creds.clientID, creds.clientSecret, code, redirectURI)
		if err != nil {
			return err
		}
		if err := saveSpotifyToken(token); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "Spotify token stored in OS keychain.")
		return nil
	case err := <-errCh:
		return err
	case <-time.After(3 * time.Minute):
		return runtimef("timed out waiting for Spotify login callback")
	}
}

type spotifyCreds struct{ clientID, clientSecret string }

func loadSpotifyCredentials() (spotifyCreds, error) {
	clientID := strings.TrimSpace(os.Getenv("WIIM_SPOTIFY_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("WIIM_SPOTIFY_CLIENT_SECRET"))
	if clientID == "" {
		clientID, _ = keyring.Get(spotifyKeyringSvc, spotifyClientIDKey)
	}
	if clientSecret == "" {
		clientSecret, _ = keyring.Get(spotifyKeyringSvc, spotifyClientSecKey)
	}
	if clientID == "" || clientSecret == "" {
		return spotifyCreds{}, usagef("Spotify client credentials not found; run `wiim spotify credentials set`")
	}
	return spotifyCreds{clientID: clientID, clientSecret: clientSecret}, nil
}

func spotifyAuthURL(clientID, state, redirectURI string) string {
	values := url.Values{}
	values.Set("client_id", clientID)
	values.Set("response_type", "code")
	values.Set("redirect_uri", redirectURI)
	values.Set("scope", spotifyScopes)
	values.Set("state", state)
	return spotifyAccountsBaseURL + "/authorize?" + values.Encode()
}

func exchangeSpotifyCode(clientID, clientSecret, code, redirectURI string) (spotifyTokenCache, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	values.Set("redirect_uri", redirectURI)
	return spotifyTokenRequest(clientID, clientSecret, values, "")
}

func refreshSpotifyToken(clientID, clientSecret, refreshToken string) (spotifyTokenCache, error) {
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", refreshToken)
	return spotifyTokenRequest(clientID, clientSecret, values, refreshToken)
}

func spotifyTokenRequest(clientID, clientSecret string, values url.Values, fallbackRefresh string) (spotifyTokenCache, error) {
	req, err := http.NewRequest("POST", spotifyAccountsBaseURL+"/api/token", strings.NewReader(values.Encode()))
	if err != nil {
		return spotifyTokenCache{}, err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := spotifyHTTPClient.Do(req)
	if err != nil {
		return spotifyTokenCache{}, runtimef("Spotify token request failed: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return spotifyTokenCache{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return spotifyTokenCache{}, runtimef("Spotify token endpoint returned HTTP %d: %s", resp.StatusCode, responseSnippet(strings.TrimSpace(string(data))))
	}
	var tr spotifyTokenResponse
	if err := json.Unmarshal(data, &tr); err != nil {
		return spotifyTokenCache{}, err
	}
	refresh := tr.RefreshToken
	if refresh == "" {
		refresh = fallbackRefresh
	}
	return spotifyTokenCache{AccessToken: tr.AccessToken, RefreshToken: refresh, TokenType: tr.TokenType, Scope: tr.Scope, ExpiresAt: time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)}, nil
}

func loadSpotifyToken() (spotifyTokenCache, error) {
	text, err := keyring.Get(spotifyKeyringSvc, spotifyTokenKey)
	if err != nil {
		return spotifyTokenCache{}, err
	}
	var token spotifyTokenCache
	if err := json.Unmarshal([]byte(text), &token); err != nil {
		return spotifyTokenCache{}, err
	}
	if token.AccessToken == "" || token.RefreshToken == "" {
		return spotifyTokenCache{}, fmt.Errorf("incomplete token cache")
	}
	return token, nil
}

func saveSpotifyToken(token spotifyTokenCache) error {
	// #nosec G117 -- storing OAuth token fields in OS keychain; not a real vulnerability
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	if err := keyring.Set(spotifyKeyringSvc, spotifyTokenKey, string(data)); err != nil {
		return runtimef("could not store Spotify token in OS keychain: %v", err)
	}
	return nil
}

func randomState() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func firstRedirectURI(values []string) string {
	if len(values) > 0 && values[0] != "" {
		return values[0]
	}
	return defaultSpotifyRedirectURI
}

func spotifyCallbackListen(redirectURI string) (string, string, error) {
	u, err := url.Parse(redirectURI)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path == "" {
		return "", "", usagef("spotifyRedirectURI must be a loopback http URL like http://127.0.0.1:19872/login")
	}
	return "127.0.0.1:" + u.Port(), u.Path, nil
}

func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL) // #nosec G204 -- fixed argv, no shell
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL) // #nosec G204 -- fixed argv, no shell
	default:
		cmd = exec.Command("xdg-open", rawURL) // #nosec G204 -- fixed argv, no shell
	}
	return cmd.Start()
}

func clipboardText() string {
	commands := [][]string{{"wl-paste", "-n"}, {"xclip", "-selection", "clipboard", "-o"}, {"xsel", "--clipboard", "--output"}, {"pbpaste"}}
	for _, parts := range commands {
		// #nosec G204 -- fixed argv, no shell
		out, err := exec.Command(parts[0], parts[1:]...).Output()
		if err == nil {
			text := strings.TrimSpace(string(out))
			if text != "" && !strings.ContainsAny(text, "\n\r\t ") {
				return text
			}
		}
	}
	return ""
}

func maskSecret(value string) string {
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
}

func spotifyURI(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "spotify:") {
		return value
	}
	if u, err := url.Parse(value); err == nil && strings.Contains(u.Host, "spotify.com") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 {
			return fmt.Sprintf("spotify:%s:%s", parts[0], parts[1])
		}
	}
	return value
}
