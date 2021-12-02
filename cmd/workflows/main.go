package main

import (
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/iskorotkov/chaos-workflows/internal/config"
	"github.com/iskorotkov/chaos-workflows/internal/handlers"
	"github.com/iskorotkov/chaos-workflows/pkg/argo"
	"github.com/iskorotkov/chaos-workflows/pkg/eventws"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"
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
	argoClient, err := argo.NewClient(cfg.ArgoServer, logger.Named("argo"))
	if err != nil {
		logger.Fatal("couldn't create reader and/or writer")
	}

	wsFactory := eventws.NewWebsocketFactory(logger.Named("websockets"))
	logger.Debugw("all dependencies were initialized",
		"argo client", argoClient,
		"websocket factory", wsFactory)

	logger.Debug("creating router")
	r := createRouter(argoClient, wsFactory, logger)
	logger.Debug("router created")

	logger.Debug("server started listening")
	if err = http.ListenAndServe(":8811", r); err != nil {
		logger.Fatal(err.Error())
	}
}

// createRouter returns configured chi router.
func createRouter(argoClient argo.Client, wsFactory eventws.WebsocketFactory, logger *zap.SugaredLogger) *chi.Mux {
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
			r.Mount("/workflows", handlers.WorkflowsRouter(argoClient, wsFactory, logger.Named("workflows")))
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
