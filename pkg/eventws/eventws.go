// Package eventws uses websocket to send workflow events.
package eventws

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/iskorotkov/chaos-workflows/pkg/event"
	"go.uber.org/zap"
)

// eventWebsocket is a websocket wrapper for sending workflow events.
type eventWebsocket struct {
	conn   *websocket.Conn
	logger *zap.SugaredLogger
}

func (ew eventWebsocket) Write(ctx context.Context, ev event.Workflow) error {
	if deadline, ok := ctx.Deadline(); ok {
		if err := ew.conn.SetWriteDeadline(deadline); err != nil {
			ew.logger.Error(err)
			return event.ErrConnectionFailed
		}
	}

	if err := ew.conn.WriteJSON(ev); err != nil {
		ew.logger.Error(err)
		return event.ErrConnectionFailed
	}

	return nil
}

func (ew eventWebsocket) Close() error {
	if err := ew.Close(); err != nil {
		ew.logger.Warnf("websocket was closed with error: %s", err)
	}
	return nil
}

type WebsocketFactory struct {
	upgrader websocket.Upgrader
	logger   *zap.SugaredLogger
}

func (wf WebsocketFactory) New(w http.ResponseWriter, r *http.Request) (event.Writer, error) {
	conn, err := wf.upgrader.Upgrade(w, r, nil)
	if err != nil {
		wf.logger.Error(err)
		return nil, event.ErrConnectionFailed
	}

	return eventWebsocket{
		conn:   conn,
		logger: wf.logger.Named("websocket"),
	}, nil
}

func (wf WebsocketFactory) Close() error {
	return nil
}

func NewWebsocketFactory(logger *zap.SugaredLogger) WebsocketFactory {
	return WebsocketFactory{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		logger: logger,
	}
}
