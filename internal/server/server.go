package server

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"mpesa-gateway/internal/config"
	customMiddleware "mpesa-gateway/internal/middleware"
	"mpesa-gateway/internal/transport/http/handlers"
)

// Server wraps the HTTP server
type Server struct {
	router  *chi.Mux
	handler *handlers.Handler
	config  *config.Config
}

// NewServer creates a new HTTP server
func NewServer(cfg *config.Config, h *handlers.Handler) *Server {
	s := &Server{
		router:  chi.NewRouter(),
		handler: h,
		config:  cfg,
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures all routes and middleware
func (s *Server) setupRoutes() {
	r := s.router

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Public health check
	r.Get("/health", s.handler.HealthCheck)

	// Protected initiate endpoint (requires internal authentication)
	r.Group(func(r chi.Router) {
		r.Use(customMiddleware.EnsureInternalAuth(s.config.InternalSecret))
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

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := ":" + s.config.ServerPort
	log.Printf("Starting HTTP server on %s", addr)

	return http.ListenAndServe(addr, s.router)
}
