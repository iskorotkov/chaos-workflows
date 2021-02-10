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

const (
	// ContextReaderFactory is a key used for storing reader factory in context.
	ContextReaderFactory = "reader-factory"
	// ContextWriterFactory is a key used for storing writer factory in context.
	ContextWriterFactory = "writer-factory"
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
func watchWS(w http.ResponseWriter, r *http.Request, logger *zap.SugaredLogger) {
	// Parse request.
	namespace, name := chi.URLParam(r, "namespace"), chi.URLParam(r, "name")
	if namespace == "" || name == "" {
		msg := "namespace and name must not be empty"
		logger.Infow(msg, "namespace", namespace, "name", name)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	logger.Infow("get request params from url", "namespace", namespace, "name", name)

	// Get dependencies.
	rf, wf, ok := getContextValues(r)
	if !ok {
		msg := "couldn't get context values from request context"
		logger.Error(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	// Prepare reader.
	reader, err := rf.New(ctx, namespace, name)
	if err != nil {
		logger.Error(err)
		http.Error(w, "couldn't start event stream", http.StatusInternalServerError)
	}
	defer closeWithLogger(reader, logger)

	// Prepare writer.
	writer, err := wf.New(w, r)
	if err != nil {
		logger.Error(err)
		http.Error(w, "couldn't start event stream", http.StatusInternalServerError)
	}
	defer closeWithLogger(writer, logger)

	transmitEvents(ctx, reader, writer, w, logger)
	logger.Info("all workflow events were processed")
}

func getContextValues(r *http.Request) (ReaderFactory, WriterFactory, bool) {
	// Get reader factory
	rf, ok := r.Context().Value(ContextReaderFactory).(ReaderFactory)
	if !ok {
		return nil, nil, false
	}

	// Get writer factory
	wf, ok := r.Context().Value(ContextWriterFactory).(WriterFactory)
	if !ok {
		return nil, nil, false
	}
	return rf, wf, true
}

// transmitEvents reads events from reader and passes them to writer.
func transmitEvents(ctx context.Context, stream event.Reader, socket event.Writer, w http.ResponseWriter, logger *zap.SugaredLogger) {
	for {
		ev, err := stream.Read()
		if err == event.ErrFinished {
			break
		} else if err != nil {
			logger.Error(err)
			http.Error(w, "error occurred while streaming workflow events", http.StatusInternalServerError)
			break
		}

		if err := socket.Write(ctx, ev); err == event.ErrFinished {
			break
		} else if err != nil {
			logger.Error(err)
			http.Error(w, "error occurred while sending event via websocket", http.StatusInternalServerError)
			break
		}
	}

	logger.Info("all workflow events were read")
}

// closeWithLogger closes io.Closer, printing an error if it failed.
func closeWithLogger(stream io.Closer, logger *zap.SugaredLogger) {
	if err := stream.Close(); err != nil {
		logger.Error(err)
	}
}
