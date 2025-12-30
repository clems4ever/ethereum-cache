package server

import (
	"context"
	"net/http"

	"github.com/clems4ever/ethereum-cache/internal/cleanup"
	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/proxy"
)

type Server struct {
	httpServer     *http.Server
	cleanupManager *cleanup.Manager
}

func New(addr string, upstreamURL string, db *database.DB, authToken string, maxSize int64, slackRatio float64) *Server {
	var cleanupManager *cleanup.Manager
	if maxSize > 0 {
		cleanupManager = cleanup.NewManager(db, maxSize, slackRatio)
	}

	handler := proxy.NewHandler(upstreamURL, db, cleanupManager)

	var finalHandler http.Handler = handler
	if authToken != "" {
		finalHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer "+authToken {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			handler.ServeHTTP(w, r)
		})
	}

	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: finalHandler,
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
