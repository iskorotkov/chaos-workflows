package handlers

import (
	"github.com/go-chi/chi"
	"go.uber.org/zap"
	"net/http"
)

type handler func(w http.ResponseWriter, r *http.Request, logger *zap.SugaredLogger)

func withLogger(handler handler, logger *zap.SugaredLogger) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		handler(writer, request, logger)
	}
}

func Router(logger *zap.SugaredLogger) http.Handler {
	r := chi.NewRouter()

	r.Get("/{namespace}/{name}", withLogger(watchWS, logger.Named("watch")))

	return r
}
