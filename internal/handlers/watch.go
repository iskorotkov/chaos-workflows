package handlers

import (
	"context"
	"github.com/go-chi/chi"
	"github.com/iskorotkov/chaos-workflows/pkg/argo"
	"github.com/iskorotkov/chaos-workflows/pkg/event"
	"github.com/iskorotkov/chaos-workflows/pkg/ws"
	"go.uber.org/zap"
	"io"
	"net/http"
	"time"
)

const (
	ContextReaderFactory = "reader-factory"
	ContextWriterFactory = "writer-factory"

	prerequisitesErr = "couldn't get all prerequisites to process the request"
)

type ReaderFactory interface {
	New(ctx context.Context, namespace, name string) (event.Reader, error)
	Close() error
}

type WriterFactory interface {
	New(w http.ResponseWriter, r *http.Request) (event.Writer, error)
	Close() error
}

func watchWS(w http.ResponseWriter, r *http.Request, logger *zap.SugaredLogger) {
	namespace, name := chi.URLParam(r, "namespace"), chi.URLParam(r, "name")
	if namespace == "" || name == "" {
		msg := "namespace and name must not be empty"
		logger.Infow(msg, "namespace", namespace, "name", name)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	logger.Infow("get request params from url", "namespace", namespace, "name", name)

	rf, ok := r.Context().Value(ContextReaderFactory).(ReaderFactory)
	if !ok {
		logger.Error("couldn't get reader factory from request context")
		http.Error(w, prerequisitesErr, http.StatusInternalServerError)
	}

	wf, ok := r.Context().Value(ContextWriterFactory).(WriterFactory)
	if !ok {
		logger.Error("couldn't get writer factory from request context")
		http.Error(w, prerequisitesErr, http.StatusInternalServerError)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	reader, err := rf.New(ctx, namespace, name)
	if err != nil {
		logger.Error(err)
		http.Error(w, "couldn't start event stream", http.StatusInternalServerError)
	}
	defer closeWithLogger(reader, logger)

	writer, err := wf.New(w, r)
	if err != nil {
		logger.Error(err)
		http.Error(w, "couldn't start event stream", http.StatusInternalServerError)
	}
	defer closeWithLogger(writer, logger)

	transmitEvents(ctx, reader, writer, w, logger)
	logger.Info("all workflow events were processed")
}

func transmitEvents(ctx context.Context, stream event.Reader, socket event.Writer, w http.ResponseWriter, logger *zap.SugaredLogger) {
	for {
		ev, err := stream.Read()
		if err == argo.ErrFinished {
			logger.Info("all workflow events were read")
			break
		} else if err != nil {
			logger.Error(err)
			http.Error(w, "error occurred while streaming workflow events", http.StatusInternalServerError)
			break
		}

		if err := socket.Write(ctx, ev); err != nil && err != ws.ErrDeadlineExceeded && err != ws.ErrContextCancelled {
			logger.Error(err)
			http.Error(w, "error occurred while sending event via websocket", http.StatusInternalServerError)
			break
		}
	}
}

func closeWithLogger(stream io.Closer, logger *zap.SugaredLogger) {
	defer func() {
		if err := stream.Close(); err != nil {
			logger.Error(err)
		}
	}()
}
