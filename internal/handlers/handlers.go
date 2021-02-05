package handlers

import (
	"github.com/go-chi/chi"
	"github.com/iskorotkov/chaos-scheduler/pkg/server"
	"go.uber.org/zap"
	"net/http"
)

func Router(logger *zap.SugaredLogger) http.Handler {
	r := chi.NewRouter()

	r.Get("/{namespace}/{name}", server.WithLogger(watchWS, logger.Named("watch")))

	return r
}
