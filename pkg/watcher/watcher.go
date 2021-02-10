package watcher

import (
	"context"
	"errors"
	"fmt"
	"github.com/argoproj/argo/pkg/apiclient/workflow"
	"github.com/iskorotkov/chaos-workflows/pkg/event"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"io"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ErrConnectionFailed      = errors.New("couldn't establish connection to gRPC server")
	ErrConnectionCloseFailed = errors.New("couldn't close connection correctly")
	ErrServiceCloseFailed    = errors.New("couldn't close service stream")
	ErrRequestFailed         = errors.New("couldn't start watching updates")
	ErrReadFailed            = errors.New("couldn't read workflow update")
	ErrFinished              = errors.New("event stream finished")
)

type Watcher struct {
	conn   *grpc.ClientConn
	logger *zap.SugaredLogger
}

func NewWatcher(url string, logger *zap.SugaredLogger) (Watcher, error) {
	logger.Info("opening watcher gRPC connection")

	conn, err := grpc.Dial(url, grpc.WithInsecure())
	if err != nil {
		logger.Errorw(err.Error(), "url", url)
		return Watcher{}, ErrConnectionFailed
	}

	return Watcher{conn: conn, logger: logger}, nil
}

func (w Watcher) Watch(ctx context.Context, namespace string, name string) (event.Reader, error) {
	client := workflow.NewWorkflowServiceClient(w.conn)

	request := &workflow.WatchWorkflowsRequest{
		Namespace: namespace,
		ListOptions: &v1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", name),
		},
	}

	service, err := client.WatchWorkflows(ctx, request)
	if err != nil {
		w.logger.Errorw(err.Error(), "selector", fmt.Sprintf("metadata.name=%s", name))
		return EventStream{}, ErrRequestFailed
	}

	return EventStream{
		service: service,
		logger:  w.logger.Named(fmt.Sprintf("%s-%s", namespace, name)),
	}, nil
}

func (w Watcher) Close() error {
	w.logger.Info("closing watcher gRPC connection")
	if err := w.conn.Close(); err != nil {
		w.logger.Error(err)
		return ErrConnectionCloseFailed
	}

	return nil
}

type EventStream struct {
	ctx     context.Context
	service workflow.WorkflowService_WatchWorkflowsClient
	logger  *zap.SugaredLogger
}

func (e EventStream) Read() (event.Event, error) {
	msg, err := e.service.Recv()
	if err == io.EOF || e.ctx.Err() != nil {
		return event.Event{}, ErrFinished
	}

	if err != nil {
		e.logger.Error(err)
		return event.Event{}, ErrReadFailed
	}

	ev, err := event.NewEvent(msg)
	if err != nil {
		e.logger.Error(err)
		return event.Event{}, err
	}

	if ev.Phase != "Running" && ev.Phase != "Pending" {
		return event.Event{}, ErrFinished
	}

	return ev, nil
}

func (e EventStream) Close() error {
	if err := e.service.CloseSend(); err != nil {
		e.logger.Error(err)
		return ErrServiceCloseFailed
	}

	return nil
}
