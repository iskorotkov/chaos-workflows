package main

import (
	"context"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/iskorotkov/chaos-workflows/internal/config"
	"github.com/iskorotkov/chaos-workflows/internal/handlers"
	"go.uber.org/zap"
	"log"
	"net/http"
	"time"
)

func main() {
	cfg, err := config.FromEnvironment()
	if err != nil {
		log.Fatal(err)
	}

	logger := createLogger(cfg)
	defer syncLogger(logger)

	logger.Infow("get config from environment", "config", cfg)

	r := createRouter(cfg, logger)
	if err = http.ListenAndServe(":8811", r); err != nil {
		logger.Fatal(err.Error())
	}
}

func contextValue(key, value interface{}) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(context.WithValue(r.Context(), key, value))
			next.ServeHTTP(w, r)
		})
	}
}

func createRouter(cfg *config.Config, logger *zap.SugaredLogger) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
	}))
	r.Use(contextValue("config", cfg))

	r.Route("/api", func(r chi.Router) {
		r.Route("/v1", func(r chi.Router) {
			r.Mount("/workflows", handlers.Router(logger.Named("workflows")))
		})
	})

	return r
}

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

func syncLogger(logger *zap.SugaredLogger) {
	err := logger.Sync()
	if err != nil {
		log.Fatal(err.Error())
	}
}
