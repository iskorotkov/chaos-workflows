// Package argo reads workflow events from argo server.
package argo

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/argoproj/argo-workflows/v3/pkg/apiclient"
	"github.com/argoproj/argo-workflows/v3/pkg/apiclient/workflow"
	"github.com/iskorotkov/chaos-workflows/pkg/event"
	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Watcher creates Argo events readers.
type Watcher struct {
	client apiclient.Client
	logger *zap.SugaredLogger
}

func NewWatcher(url string, logger *zap.SugaredLogger) (Watcher, error) {
	logger.Info("opening argo gRPC connection")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	_, apiClient, err := apiclient.NewClientFromOpts(apiclient.Opts{
		ArgoServerOpts: apiclient.ArgoServerOpts{
			URL:                url,
			Path:               "",
			Secure:             true,
			InsecureSkipVerify: true,
			HTTP1:              false,
		},
		InstanceID: "",
		AuthSupplier: func() string {
			return ""
		},
		ClientConfigSupplier: nil,
		Offline:              false,
		Context:              ctx,
	})
	if err != nil {
		logger.Errorw(err.Error(), "url", url)
		return Watcher{}, event.ErrConnectionFailed
	}

	logger.Debug("argo watcher created successfully")
	return Watcher{client: apiClient, logger: logger}, nil
}

func (w Watcher) New(ctx context.Context, namespace string, name string) (event.Reader, error) {
	service, err := w.client.NewWorkflowServiceClient().WatchWorkflows(ctx, &workflow.WatchWorkflowsRequest{
		Namespace: namespace,
		ListOptions: &v1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", name),
		},
	})
	if err != nil {
		w.logger.Errorw(err.Error(), "selector", fmt.Sprintf("metadata.name=%s", name))
		return eventStream{}, event.ErrConnectionFailed
	}

	return eventStream{
		ctx:     ctx,
		service: service,
		logger:  w.logger.Named(fmt.Sprintf("%s-%s", namespace, name)),
	}, nil
}

func (w Watcher) Close() error {
	return nil
}

// eventStream reads stream of workflow events from Argo server.
type eventStream struct {
	ctx     context.Context
	service workflow.WorkflowService_WatchWorkflowsClient
	logger  *zap.SugaredLogger
}

func (e eventStream) Read() (event.Event, error) {
	msg, err := e.service.Recv()
	if err == io.EOF {
		return event.Event{}, event.ErrAllRead
	} else if e.ctx.Err() != nil {
		return event.Event{}, event.ErrDeadlineExceeded
	} else if err != nil {
		e.logger.Error(err)
		return event.Event{}, event.ErrConnectionFailed
	}

	ev, ok := event.ToCustomEvent(msg)
	if !ok {
		e.logger.Error("couldn't convert to custom event")
		return event.Event{}, event.ErrInvalidEvent
	}

	if ev.Phase != "Running" && ev.Phase != "Pending" {
		return ev, event.ErrAllRead
	}

	return ev, nil
}

func (e eventStream) Close() error {
	if err := e.service.CloseSend(); err != nil {
		e.logger.Error(err)
		return nil
	}

	return nil
}
