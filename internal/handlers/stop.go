package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/iskorotkov/chaos-workflows/pkg/argo"
	"github.com/iskorotkov/chaos-workflows/pkg/event"
	"go.uber.org/zap"
)

func cancelWorkflow(w http.ResponseWriter, r *http.Request, client argo.Client, log *zap.SugaredLogger) {
	namespace, name := chi.URLParam(r, "namespace"), chi.URLParam(r, "name")

	if namespace == "" || name == "" {
		log.Infof("namespace or name is empty")
		http.Error(w, "namespace or name is empty", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	dto, err := client.Stop(ctx, namespace, name)
	if err != nil {
		log.Infof("error stopping workflow %s in namespace %s: %v", name, namespace, err)
		http.Error(w, "error stopping workflow", http.StatusInternalServerError)
		return
	}

	workflow, ok := event.FromWorkflow(dto)
	if !ok {
		log.Infof("error converting raw workflow to custom type")
		http.Error(w, "error converting raw workflow to custom type", http.StatusInternalServerError)
		return
	}

	b, err := json.Marshal(workflow)
	if err != nil {
		log.Infof("error marshaling workflows: %v", err)
		http.Error(w, "error marshaling workflows", http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")

	if _, err := w.Write(b); err != nil {
		log.Infof("error writing response: %v", err)
		http.Error(w, "error writing response", http.StatusInternalServerError)
		return
	}
}
