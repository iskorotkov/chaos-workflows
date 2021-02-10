package main

import (
	"context"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/iskorotkov/chaos-workflows/internal/config"
	"github.com/iskorotkov/chaos-workflows/internal/handlers"
	"github.com/iskorotkov/chaos-workflows/pkg/argo"
	"github.com/iskorotkov/chaos-workflows/pkg/eventws"
	"go.uber.org/zap"
	"log"
	"net/http"
	"time"
)

func main() {
	// Read config.
	cfg, err := config.FromEnvironment()
	if err != nil {
		log.Fatal(err)
	}

	// Prepare logger.
	logger := createLogger(cfg)
	defer syncLogger(logger)

	logger.Infow("get config from environment", "config", cfg)

	// Setup external dependencies.
	wf, rf, err := createReaderWriter(cfg.ArgoServer, logger)
	if err != nil {
		logger.Fatal("couldn't create reader and/or writer")
	}

	r := createRouter(logger)
	addContextValues(r, cfg, rf, wf)

	if err = http.ListenAndServe(":8811", r); err != nil {
		logger.Fatal(err.Error())
	}
}

func addContextValues(r *chi.Mux, cfg *config.Config, rf handlers.WriterFactory, wf handlers.ReaderFactory) {
	r.Use(contextValue("config", cfg))
	r.Use(contextValue(handlers.ContextReaderFactory, rf))
	r.Use(contextValue(handlers.ContextWriterFactory, wf))
}

// contextValue adds key-value pair to context.Context.
func contextValue(key, value interface{}) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(context.WithValue(r.Context(), key, value))
			next.ServeHTTP(w, r)
		})
	}
}

// createRouter returns configured chi router.
func createRouter(logger *zap.SugaredLogger) *chi.Mux {
	r := chi.NewRouter()
	setupMiddleware(r)
	setupRoutes(logger, r)
	return r
}

func setupRoutes(logger *zap.SugaredLogger, r *chi.Mux) chi.Router {
	return r.Route("/api", func(r chi.Router) {
		r.Route("/v1", func(r chi.Router) {
			r.Mount("/workflows", handlers.Router(logger.Named("workflows")))
		})
	})
}

func setupMiddleware(r *chi.Mux) {
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
	}))
}

// createLogger returns configured zap logger.
func createLogger(cfg *config.Config) *zap.SugaredLogger {
	var (
		logger *zap.Logger
		err    error
	)
	if cfg.Development {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}

	if err != nil {
		log.Fatal(err)
	}

	return logger.Sugar()
}

// syncLogger flushes zap logger.
func syncLogger(logger *zap.SugaredLogger) {
	err := logger.Sync()
	if err != nil {
		log.Fatal(err.Error())
	}
}

// createReaderWriter creates reader and writer for streaming workflow events.
func createReaderWriter(writerURL string, logger *zap.SugaredLogger) (handlers.ReaderFactory, handlers.WriterFactory, error) {
	readerF, err := argo.NewWatcher(writerURL, logger.Named("argo"))
	if err != nil {
		return nil, nil, err
	}

	writerF := eventws.NewWebsocketFactory()

	return readerF, writerF, err
}
