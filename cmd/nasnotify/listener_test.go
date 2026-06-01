package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"nasnotify-go/internal/app"
)

func TestResolveHTTPSocketPathUsesEnv(t *testing.T) {
	socketPath := filepath.Join("runtime", "nasnotify.sock")
	t.Setenv("UGAPP_HTTP_SOCKET", socketPath)

	if got := app.NewRuntimeHost().ResolveHTTPSocketPath(); got != socketPath {
		t.Fatalf("ResolveHTTPSocketPath() = %q; want %q", got, socketPath)
	}
}

func TestResolveHTTPSocketPathDefaultsToOfficialSocket(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "appdata")
	t.Setenv("UGAPP_DATA_DIR", dataDir)
	t.Setenv("UGAPP_HTTP_SOCKET", "")

	want := filepath.Join(dataDir, "run", "nasnotify.sock")
	if got := app.NewRuntimeHost().ResolveHTTPSocketPath(); got != want {
		t.Fatalf("ResolveHTTPSocketPath() = %q; want %q", got, want)
	}
}

func TestResolveHTTPListenAddressUsesEnv(t *testing.T) {
	t.Setenv("UGAPP_HTTP_ADDR", "127.0.0.1:0")

	if got := app.NewRuntimeHost().ResolveHTTPListenAddress(38683); got != "127.0.0.1:0" {
		t.Fatalf("ResolveHTTPListenAddress() = %q; want %q", got, "127.0.0.1:0")
	}
}

func TestResolveHTTPListenAddressUsesPortOverride(t *testing.T) {
	t.Setenv("UGAPP_HTTP_ADDR", "")

	if got := app.NewRuntimeHost().ResolveHTTPListenAddress(39001); got != ":39001" {
		t.Fatalf("ResolveHTTPListenAddress() = %q; want %q", got, ":39001")
	}
}

func TestNewHTTPListenerUsesTCPByDefault(t *testing.T) {
	t.Setenv("UGAPP_HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("UGAPP_HTTP_MODE", "")

	listener, endpoint, err := app.NewRuntimeHost().NewHTTPListener(0)
	if err != nil {
		t.Fatalf("NewHTTPListener() error = %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr = %T; want *net.TCPAddr", listener.Addr())
	}
	wantEndpoint := fmt.Sprintf("http://127.0.0.1:%d", addr.Port)
	if endpoint != wantEndpoint {
		t.Fatalf("endpoint = %q; want %q", endpoint, wantEndpoint)
	}
}

func TestNewHTTPListenerUsesExplicitUnixSocket(t *testing.T) {
	socketPath := testUnixSocketPath(t)
	t.Setenv("UGAPP_HTTP_SOCKET", socketPath)
	t.Setenv("UGAPP_HTTP_MODE", "unix")

	listener, endpoint, err := app.NewRuntimeHost().NewHTTPListener(0)
	if err != nil {
		t.Fatalf("NewHTTPListener() error = %v", err)
	}
	defer listener.Close()

	if _, ok := listener.Addr().(*net.UnixAddr); !ok {
		t.Fatalf("listener addr = %T; want *net.UnixAddr", listener.Addr())
	}
	if endpoint != "unix://"+socketPath {
		t.Fatalf("endpoint = %q; want unix socket endpoint", endpoint)
	}
	if _, err := os.Lstat(socketPath); err != nil {
		t.Fatalf("socket was not created: %v", err)
	}
}

func TestNewHTTPListenerDefaultsToOfficialUnixSocket(t *testing.T) {
	socketPath := testUnixSocketPath(t)
	t.Setenv("UGAPP_HTTP_SOCKET", socketPath)
	t.Setenv("UGAPP_HTTP_MODE", "unix")

	listener, endpoint, err := app.NewRuntimeHost().NewHTTPListener(0)
	if err != nil {
		t.Fatalf("NewHTTPListener() error = %v", err)
	}
	defer listener.Close()

	if _, ok := listener.Addr().(*net.UnixAddr); !ok {
		t.Fatalf("listener addr = %T; want *net.UnixAddr", listener.Addr())
	}
	if endpoint != "unix://"+socketPath {
		t.Fatalf("endpoint = %q; want unix socket endpoint", endpoint)
	}
}

func TestNewRouteProxyListenerUsesUnixSocketSidecar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("route proxy sidecar is only enabled for Linux packaging")
	}

	socketPath := testUnixSocketPath(t)
	t.Setenv("UGAPP_HTTP_SOCKET", socketPath)
	t.Setenv("UGAPP_HTTP_MODE", "")

	listener, endpoint, err := app.NewRuntimeHost().NewRouteProxyListener()
	if err != nil {
		t.Fatalf("NewRouteProxyListener() error = %v", err)
	}
	if listener == nil {
		t.Fatalf("NewRouteProxyListener() listener = nil; want unix listener")
	}
	defer listener.Close()

	if _, ok := listener.Addr().(*net.UnixAddr); !ok {
		t.Fatalf("listener addr = %T; want *net.UnixAddr", listener.Addr())
	}
	if endpoint != "unix://"+socketPath {
		t.Fatalf("endpoint = %q; want unix socket endpoint", endpoint)
	}
}

func TestNewRouteProxyListenerSkipsWhenPrimaryModeIsUnix(t *testing.T) {
	t.Setenv("UGAPP_HTTP_MODE", "unix")

	listener, endpoint, err := app.NewRuntimeHost().NewRouteProxyListener()
	if err != nil {
		t.Fatalf("NewRouteProxyListener() error = %v", err)
	}
	if listener != nil {
		defer listener.Close()
		t.Fatalf("NewRouteProxyListener() listener = %v; want nil", listener)
	}
	if endpoint != "" {
		t.Fatalf("endpoint = %q; want empty", endpoint)
	}
}

func testUnixSocketPath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "nn-test.sock")
	}
	return filepath.Join(t.TempDir(), "nasnotify.sock")
}
