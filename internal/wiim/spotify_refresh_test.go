package wiim

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

type refreshRoundTripper func(*http.Request) (*http.Response, error)

func (f refreshRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSpotifyRefreshFailurePreservesCredentials(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		body         string
		transportErr error
	}{
		{name: "transport", transportErr: errors.New("synthetic unavailable")},
		{name: "timeout", transportErr: &net.DNSError{IsTimeout: true}},
		{name: "server", status: 503, body: `{"error":"server_error"}`},
		{name: "rate limited", status: 429, body: `{"error":"invalid_grant"}`},
		{name: "invalid client", status: 400, body: `{"error":"invalid_client"}`},
		{name: "unauthorized", status: 401, body: `{"error":"invalid_grant"}`},
		{name: "malformed error", status: 400, body: `{"error":`},
		{name: "malformed success", status: 200, body: `{`},
		{name: "empty success", status: 200, body: `{}`},
		{name: "null success", status: 200, body: `null`},
		{name: "error success", status: 200, body: `{"error":"invalid_grant"}`},
		{name: "empty access", status: 200, body: `{"access_token":" ","token_type":"Bearer","expires_in":3600}`},
		{name: "wrong type", status: 200, body: `{"access_token":"new","token_type":"other","expires_in":3600}`},
		{name: "missing lifetime", status: 200, body: `{"access_token":"new","token_type":"Bearer"}`},
		{name: "negative lifetime", status: 200, body: `{"access_token":"new","token_type":"Bearer","expires_in":-1}`},
		{name: "overflow lifetime", status: 200, body: `{"access_token":"new","token_type":"Bearer","expires_in":9223372037}`},
	} {
		for _, reauth := range []bool{false, true} {
			name := tc.name
			if reauth {
				name += "/reauth"
			}
			t.Run(name, func(t *testing.T) {
				setupSpotifyTest(t)
				t.Setenv("WIIM_SPOTIFY_CLIENT_ID", "synthetic-client")
				t.Setenv("WIIM_SPOTIFY_CLIENT_SECRET", "synthetic-secret")
				old := spotifyTokenCache{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(-time.Minute)}
				if err := saveSpotifyToken(old); err != nil {
					t.Fatal(err)
				}
				before, err := keyring.Get(spotifyKeyringSvc, spotifyTokenKey)
				if err != nil {
					t.Fatal(err)
				}
				requests, logins := 0, 0
				spotifyHTTPClient = &http.Client{Transport: refreshRoundTripper(func(*http.Request) (*http.Response, error) {
					requests++
					if tc.transportErr != nil {
						return nil, tc.transportErr
					}
					return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
				})}
				listenSpotifyCallback = func(string, string) (net.Listener, error) { logins++; return nil, errors.New("unexpected login") }
				client, err := NewSpotifyClient(reauth, io.Discard)
				if err == nil || client != nil {
					t.Fatalf("client %v, err %v", client, err)
				}
				after, readErr := keyring.Get(spotifyKeyringSvc, spotifyTokenKey)
				if readErr != nil || after != before {
					t.Fatalf("cached credentials changed: %v", readErr)
				}
				if requests != 1 || logins != 0 {
					t.Fatalf("requests %d logins %d", requests, logins)
				}
				if strings.Contains(err.Error(), "cleared") {
					t.Fatalf("misleading error %v", err)
				}
			})
		}
	}
}

func TestSpotifyInvalidGrantAllowsReauth(t *testing.T) {
	setupSpotifyTest(t)
	t.Setenv("WIIM_SPOTIFY_CLIENT_ID", "synthetic-client")
	t.Setenv("WIIM_SPOTIFY_CLIENT_SECRET", "synthetic-secret")
	if err := saveSpotifyToken(spotifyTokenCache{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	spotifyHTTPClient = &http.Client{Transport: refreshRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"invalid_grant"}`))}, nil
	})}
	logins := 0
	sentinel := errors.New("synthetic login stopped")
	listenSpotifyCallback = func(string, string) (net.Listener, error) { logins++; return nil, sentinel }
	_, err := NewSpotifyClient(true, io.Discard)
	if !errors.Is(err, sentinel) || logins != 1 {
		t.Fatalf("login count %d err %v", logins, err)
	}
	if _, err := keyring.Get(spotifyKeyringSvc, spotifyTokenKey); err != keyring.ErrNotFound {
		t.Fatalf("invalid token retained: %v", err)
	}
}

func TestSpotifyInvalidGrantDeletionFailureStopsReauth(t *testing.T) {
	setupSpotifyTest(t)
	t.Setenv("WIIM_SPOTIFY_CLIENT_ID", "synthetic-client")
	t.Setenv("WIIM_SPOTIFY_CLIENT_SECRET", "synthetic-secret")
	if err := saveSpotifyToken(spotifyTokenCache{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	spotifyHTTPClient = &http.Client{Transport: refreshRoundTripper(func(*http.Request) (*http.Response, error) {
		keyring.MockInitWithError(errors.New("synthetic delete failure"))
		return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"invalid_grant"}`))}, nil
	})}
	logins := 0
	listenSpotifyCallback = func(string, string) (net.Listener, error) { logins++; return nil, errors.New("unexpected login") }
	_, err := NewSpotifyClient(true, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "could not delete Spotify token") || strings.Contains(err.Error(), "cleared") || logins != 0 {
		t.Fatalf("login count %d err %v", logins, err)
	}
}
