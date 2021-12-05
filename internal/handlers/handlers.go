// Package handlers describes http handling.
package handlers

import (
	"net/http"

	"github.com/go-chi/chi"
	"github.com/iskorotkov/chaos-workflows/pkg/argo"
	"github.com/iskorotkov/chaos-workflows/pkg/eventws"
	"go.uber.org/zap"
)

// WorkflowsRouter returns configured router.
func WorkflowsRouter(argoClient argo.Client, wsFactory eventws.WebsocketFactory, log *zap.SugaredLogger) http.Handler {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		listWorkflows(w, argoClient, log.Named("list"))
	})
	r.Get("/{namespace}/{name}", func(w http.ResponseWriter, r *http.Request) {
		getWorkflow(w, r, argoClient, log.Named("get"))
	})
	r.Get("/{namespace}/{name}/watch", func(w http.ResponseWriter, r *http.Request) {
		watchWS(w, r, argoClient, wsFactory, log.Named("watch"))
	})
	r.Post("/{namespace}/{name}/cancel", func(w http.ResponseWriter, r *http.Request) {
		cancelWorkflow(w, r, argoClient, log.Named("cancel"))
	})

	return r
}
