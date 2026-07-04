package wiim

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// PlayFile serves a local audio file to the WiiM by starting a temporary HTTP
// server on the LAN interface that routes to the device. It blocks until SIGINT
// or SIGTERM and then shuts down the server.
func PlayFile(client device, deviceHost, path string, stdout io.Writer) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", usagef("could not read file %s: %v", path, err)
	}
	if info.IsDir() {
		return "", usagef("play-file requires a file, not a directory")
	}

	fileServer, err := newLocalFileServer(abs, deviceHost)
	if err != nil {
		return "", err
	}
	serveErr := fileServer.serve()
	defer fileServer.close()

	if err := client.PlayURL(fileServer.mediaURL); err != nil {
		return "", err
	}
	fmt.Fprintf(stdout, "Serving %s as %s\nPress Ctrl-C to stop.\n", abs, fileServer.mediaURL)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if err != nil {
			return "", runtimef("local file server stopped unexpectedly: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := fileServer.shutdown(shutdownCtx); err != nil {
		return "", runtimef("could not stop local file server: %v", err)
	}
	return "", nil
}

type localFileServer struct {
	server   *http.Server
	listener net.Listener
	mediaURL string
}

func newLocalFileServer(abs, deviceHost string) (*localFileServer, error) {
	ip, err := localIPForDevice(deviceHost)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		return nil, runtimef("could not start local file server on %s: %v", ip, err)
	}

	token, err := randomToken()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	name := filepath.Base(abs)
	targetPath := "/" + token + "/" + url.PathEscape(name)
	hostPort := net.JoinHostPort(ip.String(), strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))
	mediaURL := "http://" + hostPort + targetPath

	server := &http.Server{Handler: fileHandler(abs, name, targetPath), ReadHeaderTimeout: 5 * time.Second}
	return &localFileServer{server: server, listener: ln, mediaURL: mediaURL}, nil
}

func (s *localFileServer) serve() <-chan error {
	errCh := make(chan error, 1)
	go func() {
		err := s.server.Serve(s.listener)
		if err == http.ErrServerClosed {
			err = nil
		}
		errCh <- err
	}()
	return errCh
}

func (s *localFileServer) shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *localFileServer) close() error {
	return s.server.Close()
}

func fileHandler(abs, name, targetPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != targetPath {
			http.NotFound(w, r)
			return
		}
		file, err := os.Open(abs)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, name, info.ModTime(), file)
	})
}

func localIPForDevice(deviceHost string) (net.IP, error) {
	host := strings.TrimSpace(deviceHost)
	if host == "" {
		return nil, runtimef("could not determine LAN IP for file server: WiiM host is empty")
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	} else {
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}

	conn, err := net.Dial("udp", net.JoinHostPort(host, "80"))
	if err != nil {
		return nil, runtimef("could not determine LAN IP for file server by routing to WiiM host %s: %v", deviceHost, err)
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil || addr.IP.IsUnspecified() {
		return nil, runtimef("could not determine LAN IP for file server by routing to WiiM host %s", deviceHost)
	}
	return addr.IP, nil
}

func randomToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", runtimef("could not generate file server token: %v", err)
	}
	return hex.EncodeToString(buf), nil
}
