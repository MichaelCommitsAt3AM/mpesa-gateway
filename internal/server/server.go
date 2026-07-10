package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/mpesa-gateway/internal/config"
	customMiddleware "github.com/mpesa-gateway/internal/middleware"
	"github.com/mpesa-gateway/internal/handlers"
	"github.com/mpesa-gateway/internal/tenant"
)

// Server wraps the HTTP server
type Server struct {
	router      *chi.Mux
	handler     *handlers.Handler
	config      *config.Config
	tenantStore *tenant.Store
	httpServer  *http.Server
}

// NewServer creates a new HTTP server
func NewServer(cfg *config.Config, h *handlers.Handler, tenantStore *tenant.Store) *Server {
	s := &Server{
		router:      chi.NewRouter(),
		handler:     h,
		config:      cfg,
		tenantStore: tenantStore,
	}

	s.setupRoutes()
	s.httpServer = &http.Server{Handler: s.router}
	return s
}

// setupRoutes configures all routes and middleware
func (s *Server) setupRoutes() {
	r := s.router

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(customMiddleware.TrustedRealIP(s.config.TrustedProxies))
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Public health check
	r.Get("/health", s.handler.HealthCheck)

	// Protected initiate endpoint (requires per-tenant API key)
	r.Group(func(r chi.Router) {
		r.Use(customMiddleware.TenantAuth(s.tenantStore))
		r.Post("/initiate", s.handler.InitiatePayment)
	})

	// Callback endpoint (IP filtered + size limited)
	r.Group(func(r chi.Router) {
		r.Use(customMiddleware.IPFilter(s.config.SafaricomIPs))
		r.Use(customMiddleware.RequestSizeLimit(s.config.MaxRequestSize))
		r.Post("/callback", s.handler.MPesaCallback)
	})

	log.Println("Routes configured successfully")
}

// Router exposes the underlying router, primarily so tests can register
// additional routes before Start/Serve is called.
func (s *Server) Router() chi.Router {
	return s.router
}

// Start starts the HTTP server on the configured port, blocking until it
// stops. On a clean Shutdown, it returns http.ErrServerClosed, which callers
// should treat as expected rather than fatal.
func (s *Server) Start() error {
	addr := ":" + s.config.ServerPort
	s.httpServer.Addr = addr
	log.Printf("Starting HTTP server on %s", addr)

	return s.httpServer.ListenAndServe()
}

// Serve is like Start but serves on a caller-provided listener instead of
// binding to the configured port. Primarily for tests that need to know the
// actual bound address (e.g. an OS-assigned port via ":0").
func (s *Server) Serve(ln net.Listener) error {
	log.Printf("Starting HTTP server on %s", ln.Addr())

	return s.httpServer.Serve(ln)
}

// Shutdown gracefully stops the HTTP server: it stops accepting new
// connections and waits for in-flight requests to complete, up to ctx's
// deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
