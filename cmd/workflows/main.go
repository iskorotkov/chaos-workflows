package main

import (
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
	"os"
	"runtime/debug"
	"time"
)

func main() {
	// Handle panics.
	defer func() {
		r := recover()
		if r != nil {
			log.Printf("panic occurred: %v", r)
			debug.PrintStack()
			os.Exit(1)
		}
	}()

	// Read config.
	cfg, err := config.FromEnvironment()
	if err != nil {
		log.Fatal(err)
	}

	// Prepare logger.
	logger := createLogger(cfg)
	defer syncLogger(logger)

	logger.Infow("config loaded from environment", "config", cfg)

	logger.Debug("setup external dependencies")
	rf, wf, err := createReaderWriter(cfg.ArgoServer, logger)
	if err != nil {
		logger.Fatal("couldn't create reader and/or writer")
	}
	logger.Debugw("all dependencies were initialized", "reader factory", rf, "writer factory", wf)

	logger.Debug("creating router")
	r := createRouter(rf, wf, logger)
	logger.Debug("router created")

	logger.Debug("server started listening")
	if err = http.ListenAndServe(":8811", r); err != nil {
		logger.Fatal(err.Error())
	}
}

// createRouter returns configured chi router.
func createRouter(rf handlers.ReaderFactory, wf handlers.WriterFactory, logger *zap.SugaredLogger) *chi.Mux {
	r := chi.NewRouter()

	logger.Debug("adding middleware")
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
	}))
	logger.Debug("middleware added")

	logger.Debug("setting routes")
	r.Route("/api", func(r chi.Router) {
		r.Route("/v1", func(r chi.Router) {
			r.Mount("/workflows", handlers.Router(rf, wf, logger.Named("workflows")))
		})
	})
	logger.Debug("routes set")

	return r
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

	writerF := eventws.NewWebsocketFactory(logger.Named("websockets"))

	return readerF, writerF, err
}
