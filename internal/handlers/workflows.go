package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/iskorotkov/chaos-workflows/pkg/argo"
	"github.com/iskorotkov/chaos-workflows/pkg/event"
	"go.uber.org/zap"
)

func listWorkflows(w http.ResponseWriter, client argo.Client, log *zap.SugaredLogger) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	workflowsDTOs, err := client.List(ctx)
	if err != nil {
		log.Infof("error listing workflows: %v", err)
		http.Error(w, "error listing workflows", http.StatusInternalServerError)
		return
	}

	var workflows []event.Workflow
	for _, dto := range workflowsDTOs {
		w, ok := event.FromWorkflow(dto)
		if !ok {
			log.Infof("skipping workflow: error converting raw workflow to custom type")
			continue
		}

		workflows = append(workflows, w)
	}

	b, err := json.Marshal(workflows)
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
