// Package handlers describes http handling.
package handlers

import (
	"github.com/go-chi/chi"
	"go.uber.org/zap"
	"net/http"
)

// Router returns configured router.
func Router(rf ReaderFactory, wf WriterFactory, logger *zap.SugaredLogger) http.Handler {
	r := chi.NewRouter()

	r.Get("/{namespace}/{name}", func(writer http.ResponseWriter, request *http.Request) {
		watchWS(writer, request, rf, wf, logger.Named("watch"))
	})

	return r
}
