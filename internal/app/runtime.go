package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"shellink/internal/config"
	"shellink/internal/httpapi"
	"shellink/internal/rpc"
	"shellink/internal/service"
	"shellink/internal/sessions"
	"shellink/internal/socketrpc"
	"shellink/internal/store"
	"shellink/internal/webhooks"
)

type Runtime struct {
	cfg        config.Config
	db         *store.Store
	bus        *sessions.Bus
	manager    *sessions.Manager
	webhooks   *webhooks.Service
	dispatcher *rpc.Dispatcher
	socket     *socketrpc.Server
	http       *http.Server

	done      chan struct{}
	serveErr  chan error
	requestMu sync.Once
	closeMu   sync.Mutex
	started   bool
	closed    bool
}

func New(cfg config.Config) *Runtime {
	return &Runtime{cfg: cfg, done: make(chan struct{}), serveErr: make(chan error, 2)}
}

func (r *Runtime) Start() error {
	r.closeMu.Lock()
	if r.started {
		r.closeMu.Unlock()
		return fmt.Errorf("Shellink runtime is already started")
	}
	r.started = true
	r.closeMu.Unlock()
	var err error
	r.db, err = store.Open(r.cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	r.bus = sessions.NewBus()
	r.manager = sessions.NewManager(r.db, r.cfg, r.bus)
	if err = r.manager.MarkStale(); err != nil {
		r.manager.CloseAll("Startup failed")
		r.db.Close()
		return fmt.Errorf("mark stale sessions: %w", err)
	}
	profiles := service.NewProfiles(r.db, r.cfg.MasterKey)
	r.webhooks = webhooks.New(r.db, r.bus)
	r.dispatcher = rpc.NewDispatcher(r.cfg, r.db, profiles, r.manager, r.webhooks, r.RequestStop)
	var httpListener net.Listener
	if r.cfg.HTTPEnabled {
		address := net.JoinHostPort(r.cfg.Host, fmt.Sprint(r.cfg.Port))
		httpListener, err = net.Listen("tcp", address)
		if err != nil {
			r.manager.CloseAll("Startup failed")
			r.db.Close()
			return fmt.Errorf("listen for HTTP: %w", err)
		}
		r.http = &http.Server{
			Addr: address, Handler: httpapi.New(r.cfg, r.dispatcher, r.manager, r.webhooks).Handler(),
			ReadHeaderTimeout: 10 * time.Second,
		}
	}
	r.socket = socketrpc.NewServer(r.cfg, r.dispatcher, r.bus, r.manager)
	address, err := r.socket.ListenAddress()
	if err != nil {
		if httpListener != nil {
			_ = httpListener.Close()
		}
		r.manager.CloseAll("Startup failed")
		r.db.Close()
		return fmt.Errorf("listen for local RPC: %w", err)
	}
	go func() {
		if serveErr := r.socket.Serve(); serveErr != nil {
			r.serveErr <- serveErr
			r.RequestStop()
		}
	}()
	if r.cfg.HTTPEnabled {
		go func() {
			if serveErr := r.http.Serve(httpListener); serveErr != nil && serveErr != http.ErrServerClosed {
				r.serveErr <- serveErr
				r.RequestStop()
			}
		}()
	}
	if err := os.WriteFile(r.cfg.PIDPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0600); err != nil {
		_ = r.socket.Close()
		if r.http != nil {
			_ = r.http.Close()
		}
		r.manager.CloseAll("Startup failed")
		_ = r.db.Close()
		return fmt.Errorf("write pid file: %w", err)
	}
	log.Printf("[Shellink] Local RPC: %s", address)
	if r.cfg.HTTPEnabled {
		log.Printf("[Shellink] HTTP: http://%s", net.JoinHostPort(r.cfg.Host, fmt.Sprint(r.cfg.Port)))
	}
	return nil
}

func (r *Runtime) RequestStop()              { r.requestMu.Do(func() { close(r.done) }) }
func (r *Runtime) Done() <-chan struct{}     { return r.done }
func (r *Runtime) ServeErrors() <-chan error { return r.serveErr }

func (r *Runtime) ServeError() error {
	select {
	case err := <-r.serveErr:
		return err
	default:
		return nil
	}
}

func (r *Runtime) Close() error {
	r.closeMu.Lock()
	if r.closed {
		r.closeMu.Unlock()
		return nil
	}
	r.closed = true
	r.closeMu.Unlock()
	var firstErr error
	if r.socket != nil {
		if err := r.socket.Close(); err != nil {
			firstErr = err
		}
	}
	if r.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := r.http.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		cancel()
	}
	if r.manager != nil {
		r.manager.CloseAll("Shellink daemon shut down")
	}
	if r.db != nil {
		if err := r.db.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := os.Remove(r.cfg.PIDPath); err != nil && !os.IsNotExist(err) && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (r *Runtime) Paths() (socket, httpURL string) {
	socket = r.cfg.SocketPath
	if r.cfg.PipeName != "" {
		socket = r.cfg.PipeName
	}
	if r.cfg.HTTPEnabled {
		host := r.cfg.Host
		if host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		httpURL = fmt.Sprintf("http://%s", net.JoinHostPort(host, fmt.Sprint(r.cfg.Port)))
	}
	return
}

func ResolveLogPath(cfg config.Config) string { return filepath.Clean(cfg.LogPath) }
