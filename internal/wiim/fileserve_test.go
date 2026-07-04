package wiim

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

func TestLocalFileServerServesOnlyTokenPath(t *testing.T) {
	filePath := writeTempMediaFile(t, "song.mp3", "test audio")
	server := startTestFileServer(t, filePath)

	resp, body := getURL(t, server.mediaURL)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token path status = %d, want %d; body %q", resp.StatusCode, http.StatusOK, body)
	}
	if body != "test audio" {
		t.Fatalf("token path body = %q, want %q", body, "test audio")
	}

	u, err := url.Parse(server.mediaURL)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	if len(parts) != 2 {
		t.Fatalf("unexpected media path %q", u.EscapedPath())
	}
	base := parts[1]

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "wrong token", path: "/wrong-token/" + base},
		{name: "absent token", path: "/" + base},
		{name: "root", path: "/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := getURL(t, "http://"+u.Host+tc.path)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want %d; body %q", resp.StatusCode, http.StatusNotFound, body)
			}
		})
	}

	if status := rawHTTPStatus(t, u.Host, "/"+parts[0]+"/../"+base); status != http.StatusNotFound {
		t.Fatalf("path traversal status = %d, want %d", status, http.StatusNotFound)
	}
}

func TestLocalFileServerBindsSpecificIP(t *testing.T) {
	filePath := writeTempMediaFile(t, "song.mp3", "test audio")
	server := startTestFileServer(t, filePath)

	addr := server.listener.Addr().(*net.TCPAddr)
	if !addr.IP.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("listener IP = %s, want 127.0.0.1", addr.IP)
	}
}

func TestLocalFileServerShutdown(t *testing.T) {
	filePath := writeTempMediaFile(t, "song.mp3", "test audio")
	server, err := newLocalFileServer(filePath, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	errCh := server.serve()

	resp, body := getURL(t, server.mediaURL)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status before shutdown = %d, want %d; body %q", resp.StatusCode, http.StatusOK, body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serve returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop after shutdown")
	}
}

func writeTempMediaFile(t *testing.T, name, content string) string {
	t.Helper()
	filePath := path.Join(t.TempDir(), name)
	if err := osWriteFile(filePath, []byte(content)); err != nil {
		t.Fatal(err)
	}
	return filePath
}

func startTestFileServer(t *testing.T, filePath string) *localFileServer {
	t.Helper()
	server, err := newLocalFileServer(filePath, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	errCh := server.serve()
	t.Cleanup(func() {
		_ = server.shutdown(context.Background())
		select {
		case <-errCh:
		case <-time.After(time.Second):
			_ = server.close()
		}
	})
	return server
}

func getURL(t *testing.T, rawURL string) (*http.Response, string) {
	t.Helper()
	// #nosec G107 -- URL targets our own httptest server
	resp, err := http.Get(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

func osWriteFile(name string, data []byte) error {
	return os.WriteFile(name, data, 0600)
}

func rawHTTPStatus(t *testing.T, addr, requestPath string) int {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = io.WriteString(conn, "GET "+requestPath+" HTTP/1.1\r\nHost: "+addr+"\r\nConnection: close\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
