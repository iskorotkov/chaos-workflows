// Package eventws uses websocket to send workflow events.
package eventws

import (
	"context"
	"github.com/iskorotkov/chaos-workflows/pkg/event"
	"github.com/iskorotkov/chaos-workflows/pkg/ws"
	"go.uber.org/zap"
	"net/http"
)

// eventWebsocket is a websocket wrapper for sending workflow events.
type eventWebsocket ws.Websocket

func (ew eventWebsocket) Write(ctx context.Context, ev event.Event) error {
	if err := ws.Websocket(ew).Write(ctx, ev); err == ws.ErrDeadlineExceeded || err == ws.ErrContextCancelled {
		return event.ErrFinished
	} else if err != nil {
		return event.ErrInternalFailure
	}
	return nil
}

func (ew eventWebsocket) Close() error {
	if err := ws.Websocket(ew).Close(); err == ws.ErrDeadlineExceeded || err == ws.ErrContextCancelled {
		return event.ErrFinished
	} else if err != nil {
		return event.ErrInternalFailure
	}
	return nil
}

type WebsocketFactory struct {
	logger *zap.SugaredLogger
}

func (wf WebsocketFactory) New(w http.ResponseWriter, r *http.Request) (event.Writer, error) {
	socket, err := ws.NewWebsocket(w, r, wf.logger.Named("websocket"))
	if err != nil {
		wf.logger.Error(err)
		return nil, event.ErrInternalFailure
	}

	return eventWebsocket(socket), nil
}

func (wf WebsocketFactory) Close() error {
	return nil
}

func NewWebsocketFactory(logger *zap.SugaredLogger) WebsocketFactory {
	return WebsocketFactory{logger: logger}
}
