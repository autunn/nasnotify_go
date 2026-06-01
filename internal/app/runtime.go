package app

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"nasnotify-go/internal/appenv"
)

const (
	runtimeLogFileName    = "nasnotify.log"
	defaultHTTPListenPort = 5080
	defaultHTTPSocketName = "nasnotify.sock"
)

const (
	DefaultHTTPListenPortForTest = defaultHTTPListenPort
)

type RuntimeHost struct{}

func NewRuntimeHost() *RuntimeHost {
	return &RuntimeHost{}
}

func (r *RuntimeHost) EnsureRuntimeDirs() {
	dataDir := r.DataDir()
	logDir := r.LogDir()

	for _, dir := range []string{
		filepath.Join(dataDir, "config"),
		logDir,
		filepath.Join(dataDir, "token"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			log.Printf("create runtime dir failed (%s): %v", dir, err)
		}
	}

	socketDir := filepath.Join(dataDir, "run")
	if err := os.MkdirAll(socketDir, 0o755); err != nil {
		log.Printf("create runtime dir failed (%s): %v", socketDir, err)
		return
	}
	if err := ensurePathAccessible(dataDir, socketDir); err != nil {
		log.Printf("set runtime socket permissions failed (%s): %v", socketDir, err)
	}
}

func (r *RuntimeHost) DataDir() string { return appenv.DataDir() }
func (r *RuntimeHost) LogDir() string  { return appenv.LogDir() }

func (r *RuntimeHost) ResolveHTTPSocketPath() string {
	if socketPath := strings.TrimSpace(os.Getenv("UGAPP_HTTP_SOCKET")); socketPath != "" {
		return socketPath
	}
	return filepath.Join(r.DataDir(), "run", defaultHTTPSocketName)
}

func (r *RuntimeHost) ResolveHTTPListenAddress(portOverride int) string {
	if address := strings.TrimSpace(os.Getenv("UGAPP_HTTP_ADDR")); address != "" {
		return address
	}
	port := portOverride
	if port <= 0 {
		port = defaultHTTPListenPort
	}
	return ":" + strconv.Itoa(port)
}

func (r *RuntimeHost) NewHTTPListener(portOverride int) (net.Listener, string, error) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("UGAPP_HTTP_MODE")), "unix") {
		return r.NewUnixSocketListener()
	}
	return r.NewTCPListener(portOverride)
}

func (r *RuntimeHost) NewRouteProxyListener() (net.Listener, string, error) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("UGAPP_HTTP_MODE")), "unix") {
		return nil, "", nil
	}
	if runtime.GOOS == "windows" {
		return nil, "", nil
	}
	return r.NewUnixSocketListener()
}

func (r *RuntimeHost) NewTCPListener(portOverride int) (net.Listener, string, error) {
	address := r.ResolveHTTPListenAddress(portOverride)
	if address == "" {
		return nil, "", fmt.Errorf("http listen address is empty")
	}
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return nil, "", fmt.Errorf("listen http tcp failed: %w", err)
	}
	return listener, "http://" + listener.Addr().String(), nil
}

func (r *RuntimeHost) NewUnixSocketListener() (net.Listener, string, error) {
	socketPath := r.ResolveHTTPSocketPath()
	if socketPath == "" {
		return nil, "", fmt.Errorf("unix socket path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, "", fmt.Errorf("create socket dir failed: %w", err)
	}
	if err := ensurePathAccessible(r.DataDir(), filepath.Dir(socketPath)); err != nil {
		return nil, "", fmt.Errorf("set socket dir permissions failed: %w", err)
	}
	if err := removeStaleSocket(socketPath); err != nil {
		return nil, "", err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, "", fmt.Errorf("listen unix socket failed: %w", err)
	}
	if err := os.Chmod(socketPath, 0o666); err != nil {
		_ = listener.Close()
		return nil, "", fmt.Errorf("chmod unix socket failed: %w", err)
	}
	return listener, "unix://" + socketPath, nil
}

func ensurePathAccessible(baseDir, targetDir string) error {
	baseDir = filepath.Clean(strings.TrimSpace(baseDir))
	targetDir = filepath.Clean(strings.TrimSpace(targetDir))
	if baseDir == "" || targetDir == "" {
		return nil
	}

	rel, err := filepath.Rel(baseDir, targetDir)
	if err != nil {
		return nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return nil
	}

	current := baseDir
	if err := os.Chmod(current, 0o755); err != nil {
		return err
	}
	if rel == "." {
		return nil
	}

	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		if err := os.Chmod(current, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func removeStaleSocket(socketPath string) error {
	info, err := os.Lstat(socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat unix socket failed: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("unix socket path is a directory: %s", socketPath)
	}
	if err := os.Remove(socketPath); err != nil {
		return fmt.Errorf("remove stale unix socket failed: %w", err)
	}
	return nil
}

func setupLogging(logDir string) func() {
	logPath := filepath.Join(logDir, runtimeLogFileName)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		log.Printf("open runtime log file failed (%s): %v", logPath, err)
		return func() {}
	}

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	return func() { _ = logFile.Close() }
}
