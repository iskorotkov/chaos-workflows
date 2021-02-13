package handlers

import (
	"context"
	"github.com/go-chi/chi"
	"github.com/iskorotkov/chaos-workflows/pkg/event"
	"go.uber.org/zap"
	"io"
	"net/http"
	"time"
)

// ReaderFactory creates workflow events readers.
type ReaderFactory interface {
	New(ctx context.Context, namespace, name string) (event.Reader, error)
	Close() error
}

// WriterFactory creates workflow events writers.
type WriterFactory interface {
	New(w http.ResponseWriter, r *http.Request) (event.Writer, error)
	Close() error
}

// watchWS handles requests to watch workflow events.
func watchWS(w http.ResponseWriter, r *http.Request, rf ReaderFactory, wf WriterFactory, logger *zap.SugaredLogger) {
	logger.Debug("parse request")
	namespace, name := chi.URLParam(r, "namespace"), chi.URLParam(r, "name")
	if namespace == "" || name == "" {
		msg := "namespace and name must not be empty"
		logger.Infow(msg, "namespace", namespace, "name", name)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	logger.Infow("get request params from url", "namespace", namespace, "name", name)

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	logger.Debug("prepare reader")
	reader, err := rf.New(ctx, namespace, name)
	if err != nil {
		logger.Error(err)
		http.Error(w, "couldn't start event stream", http.StatusInternalServerError)
	}
	defer closeWithLogger(reader, logger)

	logger.Debug("prepare writer")
	writer, err := wf.New(w, r)
	if err != nil {
		logger.Error(err)
		http.Error(w, "couldn't start event stream", http.StatusInternalServerError)
	}
	defer closeWithLogger(writer, logger)

	transmitEvents(ctx, reader, writer, w, logger)
	logger.Info("all workflow events were processed")
}

// transmitEvents reads events from reader and passes them to writer.
func transmitEvents(ctx context.Context, reader event.Reader, writer event.Writer, w http.ResponseWriter, logger *zap.SugaredLogger) {
	defer logger.Info("all workflow events were read")

	for {
		select {
		case <-ctx.Done():
			logger.Debug("context was cancelled while transmitting workflow events")
			return
		default:
			ev, err := reader.Read()
			if err == event.ErrFinished {
				return
			} else if err != nil {
				logger.Error(err)
				http.Error(w, "error occurred while streaming workflow events", http.StatusInternalServerError)
				return
			}

			if err := writer.Write(ctx, ev); err == event.ErrFinished {
				return
			} else if err != nil {
				logger.Error(err)
				http.Error(w, "error occurred while sending event via websocket", http.StatusInternalServerError)
				return
			}
		}
	}
}

// closeWithLogger closes io.Closer, printing an error if it failed.
func closeWithLogger(stream io.Closer, logger *zap.SugaredLogger) {
	if err := stream.Close(); err != nil {
		logger.Error(err)
	}
}
