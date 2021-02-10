package eventws

import (
	"context"
	"errors"
	"github.com/iskorotkov/chaos-workflows/pkg/event"
	"github.com/iskorotkov/chaos-workflows/pkg/ws"
	"go.uber.org/zap"
	"net/http"
)

var (
	ErrWebsocket   = errors.New("couldn't open a websocket")
	ErrWriteFailed = errors.New("couldn't write to websocket")
	ErrCloseFailed = errors.New("couldn't cleanly close websocket")
)

type eventWebsocket ws.Websocket

func (ew eventWebsocket) Write(ctx context.Context, ev event.Event) error {
	if err := ws.Websocket(ew).Write(ctx, ev); err == ws.ErrDeadlineExceeded || err == ws.ErrContextCancelled {
		return nil
	} else if err != nil {
		return ErrWriteFailed
	}
	return nil
}

func (ew eventWebsocket) Close() error {
	if err := ws.Websocket(ew).Close(); err == ws.ErrDeadlineExceeded || err == ws.ErrContextCancelled {
		return nil
	} else if err != nil {
		return ErrCloseFailed
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
		return nil, ErrWebsocket
	}

	return eventWebsocket(socket), nil
}

func (wf WebsocketFactory) Close() error {
	return nil
}

func NewWebsocketFactory() WebsocketFactory {
	return WebsocketFactory{}
}
