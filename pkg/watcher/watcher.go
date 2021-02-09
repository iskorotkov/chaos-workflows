package watcher

import (
	"context"
	"errors"
	"fmt"
	"github.com/argoproj/argo/pkg/apiclient/workflow"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"io"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ErrConnection = errors.New("couldn't establish connection to gRPC server")
	ErrRequest    = errors.New("couldn't start watching updates")
	ErrStream     = errors.New("couldn't read workflow update")
	ErrSpec       = errors.New("couldn't find step spec")
)

type Watcher struct {
	url    string
	logger *zap.SugaredLogger
}

func NewWatcher(url string, logger *zap.SugaredLogger) Watcher {
	return Watcher{url: url, logger: logger}
}

func (w Watcher) Start(ctx context.Context, name string, namespace string, output chan<- *Event) error {
	w.logger.Info("opening watcher gRPC connection")

	conn, err := grpc.Dial(w.url, grpc.WithInsecure())
	if err != nil {
		w.logger.Errorw(err.Error(),
			"url", w.url)
		return ErrConnection
	}

	defer func() {
		w.logger.Info("closing watcher gRPC connection")
		err := conn.Close()
		if err != nil {
			w.logger.Error(err.Error())
		}
	}()

	client := workflow.NewWorkflowServiceClient(conn)

	selector := fmt.Sprintf("metadata.name=%s", name)
	options := &v1.ListOptions{FieldSelector: selector}
	request := &workflow.WatchWorkflowsRequest{Namespace: namespace, ListOptions: options}
	service, err := client.WatchWorkflows(ctx, request)
	if err != nil {
		w.logger.Errorw(err.Error(),
			"selector", selector)
		return ErrRequest
	}

	defer close(output)

	for {
		msg, err := service.Recv()
		if err == io.EOF || ctx.Err() != nil {
			break
		}

		if err != nil {
			w.logger.Errorw(err.Error(),
				"selector", selector,
				"namespace", namespace)
			return ErrStream
		}

		ev, err := newEvent(msg)
		if err != nil {
			w.logger.Error(err.Error())
			return err
		}

		output <- ev

		if ev.Phase != "Running" && ev.Phase != "Pending" {
			break
		}
	}

	return nil
}
