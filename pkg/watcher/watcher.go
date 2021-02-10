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
	ErrConnectionFailed      = errors.New("couldn't establish connection to gRPC server")
	ErrConnectionCloseFailed = errors.New("couldn't close connection correctly")
	ErrServiceCloseFailed    = errors.New("couldn't close service stream")
	ErrRequestFailed         = errors.New("couldn't start watching updates")
	ErrReadFailed            = errors.New("couldn't read workflow update")
	ErrSpecAnalysisFailed    = errors.New("couldn't find step spec")
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

func (w Watcher) Watch(ctx context.Context, namespace string, name string) (EventStream, error) {
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

func (e EventStream) Next() (Event, error) {
	msg, err := e.service.Recv()
	if err == io.EOF || e.ctx.Err() != nil {
		return Event{}, ErrFinished
	}

	if err != nil {
		e.logger.Error(err)
		return Event{}, ErrReadFailed
	}

	event, err := newEvent(msg)
	if err != nil {
		e.logger.Error(err)
		return Event{}, err
	}

	if event.Phase != "Running" && event.Phase != "Pending" {
		return Event{}, ErrFinished
	}

	return event, nil
}

func (e EventStream) Close() error {
	if err := e.service.CloseSend(); err != nil {
		e.logger.Error(err)
		return ErrServiceCloseFailed
	}

	return nil
}
