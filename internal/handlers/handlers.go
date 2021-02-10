// Package handlers describes http handling.
package handlers

import (
	"github.com/go-chi/chi"
	"go.uber.org/zap"
	"net/http"
)

type handler func(w http.ResponseWriter, r *http.Request, logger *zap.SugaredLogger)

// withLogger passes additional zap.SugaredLogger to a http.HandlerFunc.
func withLogger(handler handler, logger *zap.SugaredLogger) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		handler(writer, request, logger)
	}
}

// Router returns configured router.
func Router(logger *zap.SugaredLogger) http.Handler {
	r := chi.NewRouter()

	r.Get("/{namespace}/{name}", withLogger(watchWS, logger.Named("watch")))

	return r
}
