package server

import (
	"context"
	"net/http"

	"github.com/clems4ever/ethereum-cache/internal/cleanup"
	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/proxy"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

type Server struct {
	logger         *zap.Logger
	httpServer     *http.Server
	cleanupManager *cleanup.Manager
}

func New(logger *zap.Logger, addr string, upstreamURL string, db *database.DB, authToken string, maxSize int64, slackRatio float64, rateLimit float64) *Server {
	var cleanupManager *cleanup.Manager
	if maxSize > 0 {
		cleanupManager = cleanup.NewManager(logger, db, maxSize, slackRatio)
	}

	handler := proxy.NewHandler(logger, upstreamURL, db, cleanupManager, rateLimit)

	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	r.Group(func(r chi.Router) {
		if authToken != "" {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					authHeader := r.Header.Get("Authorization")
					if authHeader != "Bearer "+authToken {
						http.Error(w, "Unauthorized", http.StatusUnauthorized)
						return
					}
					next.ServeHTTP(w, r)
				})
			})
		}

		r.Handle("/metrics", promhttp.Handler())
		r.Mount("/", handler)
	})

	return &Server{
		logger: logger,
		httpServer: &http.Server{
			Addr:    addr,
			Handler: r,
		},
		cleanupManager: cleanupManager,
	}
}

func (s *Server) Start() error {
	if s.cleanupManager != nil {
		s.cleanupManager.Start()
	}
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.cleanupManager != nil {
		s.cleanupManager.Stop()
	}
	return s.httpServer.Shutdown(ctx)
}
