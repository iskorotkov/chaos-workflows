// Package ws is a wrapper for websocket from github.com/gobwas/ws.
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"
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

// Websocket is a websocket wrapper.
type Websocket struct {
	conn   net.Conn
	logger *zap.SugaredLogger
}

func NewWebsocket(w http.ResponseWriter, r *http.Request, logger *zap.SugaredLogger) (Websocket, error) {
	logger.Info("opening websocket connection")

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		logger.Error(err.Error())
		return Websocket{}, ErrConnection
	}

	return Websocket{conn: conn, logger: logger}, nil
}

// Read deserializes message from websocket to data.
// The method is blocking.
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

	// Read frame.
	header, err := reader.NextFrame()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		w.logger.Info("websocket connection deadline exceeded")
		return ErrDeadlineExceeded
	} else if err != nil {
		w.logger.Error(err)
		return ErrRead
	}

	// Check websocket status.
	if header.OpCode == ws.OpClose {
		w.logger.Info("couldn't read message from websocket due to EOF")
		return ErrEOF
	}

	// Decode message.
	if err := decoder.Decode(&data); err != nil {
		w.logger.Error(err)
		return ErrDecode
	}

	return nil
}

// Write serializes data to websocket.
// The method is blocking.
func (w Websocket) Write(ctx context.Context, data interface{}) error {
	if ctx.Err() == context.Canceled {
		return ErrContextCancelled
	} else if ctx.Err() != nil {
		return ErrDeadlineExceeded
	}

	if err := w.setDeadline(ctx); err != nil {
		return err
	}

	writer := wsutil.NewWriter(w.conn, ws.StateServerSide, ws.OpText)
	encoder := json.NewEncoder(writer)

	// Prepare message.
	if err := encoder.Encode(&data); err != nil {
		w.logger.Error(err)
		return ErrEncode
	}

	// Write message to websocket.
	if err := writer.Flush(); errors.Is(err, os.ErrDeadlineExceeded) {
		w.logger.Info("websocket connection deadline exceeded")
		return ErrDeadlineExceeded
	} else if err != nil {
		w.logger.Error(err)
		return ErrFlush
	}

	return nil
}

func (w Websocket) Close() error {
	w.logger.Infow("closing websocket connection")

	if err := w.conn.Close(); errors.Is(err, os.ErrDeadlineExceeded) {
		w.logger.Info("websocket connection deadline exceeded")
		return ErrDeadlineExceeded
	} else if err != nil {
		w.logger.Error(err)
		return ErrClose
	}

	return nil
}

func (w Websocket) setDeadline(ctx context.Context) error {
	t, ok := ctx.Deadline()
	if !ok {
		// Reset deadline.
		t = time.Time{}
	}

	if err := w.conn.SetDeadline(t); err != nil {
		w.logger.Error(err)
		return ErrDeadlineSetting
	}

	return nil
}
