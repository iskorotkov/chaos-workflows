package handlers

import (
	"context"
	"github.com/go-chi/chi"
	"github.com/iskorotkov/chaos-workflows/internal/config"
	"github.com/iskorotkov/chaos-workflows/pkg/watcher"
	"github.com/iskorotkov/chaos-workflows/pkg/ws"
	"go.uber.org/zap"
	"net/http"
	"time"
)

func watchWS(w http.ResponseWriter, r *http.Request, logger *zap.SugaredLogger) {
	namespace, name := chi.URLParam(r, "namespace"), chi.URLParam(r, "name")
	if namespace == "" || name == "" {
		msg := "namespace and name must not be empty"
		logger.Infow(msg, "namespace", namespace, "name", name)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	logger.Infow("get request params from url", "namespace", namespace, "name", name)

	cfg, ok := r.Context().Value("config").(*config.Config)
	if !ok {
		msg := "couldn't get config for request context"
		logger.Error(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	// TODO: use interface
	socket, err := ws.NewWebsocket(w, r, logger.Named("websocket"))
	if err != nil {
		logger.Error(err)
		http.Error(w, "couldn't create websocket connection", http.StatusInternalServerError)
		return
	}
	defer closeWebsocket(socket, logger)

	// TODO: use interface
	wt, err := watcher.NewWatcher(cfg.ArgoServer, logger.Named("monitor"))
	if err != nil {
		logger.Error(err)
		http.Error(w, "couldn't create event watcher", http.StatusInternalServerError)
		return
	}
	defer closeWatcher(wt, logger)

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	stream, err := wt.Watch(ctx, namespace, name)
	if err != nil {
		logger.Error(err)
		http.Error(w, "couldn't start event stream", http.StatusInternalServerError)
	}
	defer closeStream(stream, logger)

	transmitEvents(ctx, stream, socket, w, logger)
	logger.Info("all workflow events were processed")
}

func transmitEvents(ctx context.Context, stream watcher.EventStream, socket ws.Websocket, w http.ResponseWriter, logger *zap.SugaredLogger) {
	for {
		event, err := stream.Next()
		if err == watcher.ErrFinished {
			logger.Info("all workflow events were read")
			break
		} else if err != nil {
			logger.Error(err)
			http.Error(w, "error occurred while streaming workflow events", http.StatusInternalServerError)
			break
		}

		if err := socket.Write(ctx, event); err != nil && err != ws.ErrDeadlineExceeded && err != ws.ErrContextCancelled {
			logger.Error(err)
			http.Error(w, "error occurred while sending event via websocket", http.StatusInternalServerError)
			break
		}
	}
}

func closeStream(stream watcher.EventStream, logger *zap.SugaredLogger) {
	if err := stream.Close(); err != nil {
		logger.Error(err)
	}
}

func closeWebsocket(socket ws.Websocket, logger *zap.SugaredLogger) {
	if err := socket.Close(); err != nil && err != ws.ErrDeadlineExceeded {
		logger.Error(err.Error())
	}
}

func closeWatcher(wt watcher.Watcher, logger *zap.SugaredLogger) {
	if err := wt.Close(); err != nil {
		logger.Error(err)
	}
}
