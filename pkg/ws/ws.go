package ws

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

var (
	ErrConnection       = errors.New("couldn't upgrade websocket connection")
	ErrDeadlineSetting  = errors.New("couldn't set deadline")
	ErrDeadlineExceeded = errors.New("connection deadline exceeded")
	ErrContextCancelled = errors.New("context was cancelled")
	ErrRead             = errors.New("couldn't read next message")
	ErrEOF              = errors.New("read all messages")
	ErrDecode           = errors.New("couldn't decode json message")
	ErrEncode           = errors.New("couldn't encode json message")
	ErrFlush            = errors.New("couldn't flush encoded message")
	ErrClose            = errors.New("couldn't close websocket connection")
)

type CloseReason string

var (
	ReasonErrorOccurred    = CloseReason("error occurred while waiting for connection closing")
	ReasonDeadlineExceeded = CloseReason("websocket connection deadline exceeded")
	ReasonClosedOnServer   = CloseReason("websocket connection was closed on the server")
	ReasonClosedOnClient   = CloseReason("websocket connection was closed on the client")
	ReasonEOF              = CloseReason("websocket was closed due to EOF caused by disconnected client")
)

type Websocket struct {
	conn   net.Conn
	Closed <-chan CloseReason
	logger *zap.SugaredLogger
}

func NewWebsocket(w http.ResponseWriter, r *http.Request, logger *zap.SugaredLogger) (Websocket, error) {
	logger.Info("opening websocket connection")

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		logger.Error(err.Error())
		return Websocket{}, ErrConnection
	}

	ch := make(chan CloseReason, 1)

	socket := Websocket{conn: conn, Closed: ch, logger: logger}

	go func() {
		defer close(ch)
		ch <- socket.waitForClosing()
	}()

	return socket, nil
}

func (w Websocket) Read(ctx context.Context, data interface{}) error {
	if ctx.Err() != nil {
		if ctx.Err() == context.Canceled {
			return ErrContextCancelled
		} else {
			return ErrDeadlineExceeded
		}
	}

	if err := w.setDeadline(ctx); err != nil {
		return err
	}

	reader := wsutil.NewReader(w.conn, ws.StateServerSide)
	decoder := json.NewDecoder(reader)

	header, err := reader.NextFrame()
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			w.logger.Info("websocket connection deadline exceeded")
			return ErrDeadlineExceeded
		}

		w.logger.Error(err)
		return ErrRead
	}

	if header.OpCode == ws.OpClose {
		w.logger.Error("couldn't read message from websocket due to EOF")
		return ErrEOF
	}

	if err := decoder.Decode(&data); err != nil {
		w.logger.Error(err)
		return ErrDecode
	}

	return nil
}

func (w Websocket) Write(ctx context.Context, data interface{}) error {
	if ctx.Err() != nil {
		if ctx.Err() == context.Canceled {
			return ErrContextCancelled
		} else {
			return ErrDeadlineExceeded
		}
	}

	if err := w.setDeadline(ctx); err != nil {
		return err
	}

	writer := wsutil.NewWriter(w.conn, ws.StateServerSide, ws.OpText)
	encoder := json.NewEncoder(writer)

	if err := encoder.Encode(&data); err != nil {
		w.logger.Error(err.Error())
		return ErrEncode
	}

	if err := writer.Flush(); err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			w.logger.Info("websocket connection deadline exceeded")
			return ErrDeadlineExceeded
		}

		w.logger.Error(err.Error())
		return ErrFlush
	}

	return nil
}

func (w Websocket) Close() error {
	w.logger.Infow("closing websocket connection")

	err := w.conn.Close()
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			w.logger.Info("websocket connection deadline exceeded")
			return ErrDeadlineExceeded
		}

		w.logger.Error(err.Error())
		return ErrClose
	}

	return nil
}

func (w Websocket) setDeadline(ctx context.Context) error {
	t, ok := ctx.Deadline()
	if !ok {
		t = time.Time{}
	}

	if err := w.conn.SetDeadline(t); err != nil {
		w.logger.Error(err)
		return ErrDeadlineSetting
	}

	return nil
}

func (w Websocket) waitForClosing() CloseReason {
	for {
		header, err := ws.ReadHeader(w.conn)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return ReasonDeadlineExceeded
			}

			if _, ok := err.(*net.OpError); ok {
				return ReasonClosedOnServer
			}

			if err == io.EOF {
				return ReasonEOF
			}

			w.logger.Warn(err.Error())
			return ReasonErrorOccurred
		}

		if header.OpCode == ws.OpClose {
			return ReasonClosedOnClient
		}
	}
}
