package app

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"nasnotify-go/internal/config"
	"nasnotify-go/internal/notify"
)

type App struct {
	version string
	port    int
	http    *HTTPGateway
	runtime *RuntimeHost
	tasks   *TaskRuntime
}

func New(version string, port int) *App {
	return &App{
		version: version,
		port:    port,
		http:    NewHTTPGateway(version),
		runtime: NewRuntimeHost(),
		tasks:   NewTaskRuntime(),
	}
}

func (a *App) Run() error {
	recordStartupEvent("process entered main")
	a.runtime.EnsureRuntimeDirs()
	config.InitConfig()
	migrateLegacyAdminPassword()

	cleanupLogging := setupLogging(a.runtime.LogDir())
	defer cleanupLogging()

	if !config.IsInitialized() {
		ensureSetupToken()
	} else {
		go func() {
			if err := notify.CreateEnterpriseWechatMenu(); err != nil {
				log.Printf("sync enterprise wechat menu failed: %v", err)
			}
		}()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var loopWG sync.WaitGroup
	desktopIntegration := NewDesktopIntegration(a.runtime)
	desktopIntegration.Start(ctx, &loopWG)
	defer desktopIntegration.Close()

	a.tasks.Start(ctx, &loopWG)

	appServer := newHTTPServer(a.http.NewRouter())

	listener, listenEndpoint, err := a.runtime.NewHTTPListener(a.port)
	if err != nil {
		fatalStartup("create http listener failed: %v", err)
	}
	bindings := []listenerBinding{{
		listener: listener,
		endpoint: listenEndpoint,
	}}

	if routeListener, routeEndpoint, routeErr := a.runtime.NewRouteProxyListener(); routeErr != nil {
		log.Printf("route proxy listener unavailable: %v", routeErr)
	} else if routeListener != nil {
		bindings = append(bindings, listenerBinding{
			listener: routeListener,
			endpoint: routeEndpoint,
		})
	}

	defer closeListeners(bindings)

	serverErrCh := make(chan error, len(bindings))
	for _, binding := range bindings {
		go func(active listenerBinding) {
			serverErrCh <- appServer.Serve(active.listener)
		}(binding)
	}

	log.Printf("NasNotify %s started on %s", a.version, joinListenerEndpoints(bindings))
	notifyServiceManagerReady()

	select {
	case err := <-serverErrCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		waitBackgroundLoops(&loopWG, 3*time.Second)
		return nil
	case <-ctx.Done():
		log.Printf("shutdown signal received: %v", ctx.Err())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := appServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("app server shutdown failed: %v", err)
	}

	waitBackgroundLoops(&loopWG, 3*time.Second)
	log.Printf("NasNotify %s stopped", a.version)
	return nil
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

type listenerBinding struct {
	listener net.Listener
	endpoint string
}

func closeListeners(bindings []listenerBinding) {
	for _, binding := range bindings {
		_ = binding.listener.Close()
	}
}

func joinListenerEndpoints(bindings []listenerBinding) string {
	values := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if binding.endpoint == "" {
			continue
		}
		values = append(values, binding.endpoint)
	}
	return strings.Join(values, ", ")
}
