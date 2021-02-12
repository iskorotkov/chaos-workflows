package handlers

import (
	"context"
	"fmt"
	"github.com/go-chi/chi"
	"github.com/iskorotkov/chaos-workflows/pkg/event"
	"go.uber.org/zap"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"testing/quick"
)

// TestReaderFactory mocks ReaderFactory.
type TestReaderFactory struct {
	Namespace, Name string

	Reader *event.TestReader
	ErrNew error

	Closed   bool
	ErrClose error
}

func (t TestReaderFactory) Generate(rand *rand.Rand, size int) reflect.Value {
	reader := event.TestReader{}.Generate(rand, size).Interface().(event.TestReader)
	return reflect.ValueOf(TestReaderFactory{
		Reader:   &reader,
		ErrNew:   event.GenerateTestError(rand),
		ErrClose: event.GenerateTestError(rand),
	})
}

func (t *TestReaderFactory) New(_ context.Context, namespace, name string) (event.Reader, error) {
	t.Namespace, t.Name = namespace, name
	return t.Reader, t.ErrNew
}

func (t *TestReaderFactory) Close() error {
	t.Closed = true
	return t.ErrClose
}

// TestWriterFactory mocks WriterFactory.
type TestWriterFactory struct {
	Writer *event.TestWriter
	ErrNew error

	Closed   bool
	ErrClose error
}

func (t TestWriterFactory) Generate(rand *rand.Rand, size int) reflect.Value {
	writer := event.TestWriter{}.Generate(rand, size).Interface().(event.TestWriter)
	return reflect.ValueOf(TestWriterFactory{
		Writer:   &writer,
		ErrNew:   event.GenerateTestError(rand),
		ErrClose: event.GenerateTestError(rand),
	})
}

func (t *TestWriterFactory) New(_ http.ResponseWriter, _ *http.Request) (event.Writer, error) {
	return t.Writer, t.ErrNew
}

func (t *TestWriterFactory) Close() error {
	t.Closed = true
	return t.ErrClose
}

// Test_watchWS tests handler for watching workflow events.
func Test_watchWS(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(0))

	EOFError := func(readerFactory TestReaderFactory, writerFactory TestWriterFactory) bool {
		return readerFactory.Reader.ErrRead == event.ErrFinished ||
			writerFactory.Writer.ErrWrite == event.ErrFinished
	}

	readWriteError := func(readerFactory TestReaderFactory, writerFactory TestWriterFactory) bool {
		return readerFactory.ErrNew != nil ||
			writerFactory.ErrNew != nil ||
			readerFactory.Reader.ErrRead != nil && readerFactory.Reader.ErrRead != event.ErrFinished ||
			writerFactory.Writer.ErrWrite != nil && writerFactory.Writer.ErrWrite != event.ErrFinished
	}

	closeError := func(readerFactory TestReaderFactory, writerFactory TestWriterFactory) bool {
		err := readerFactory.ErrClose != nil ||
			writerFactory.ErrClose != nil ||
			readerFactory.Reader.ErrClose != nil ||
			writerFactory.Writer.ErrClose != nil
		return err
	}

	f := func(readerFactory TestReaderFactory, writerFactory TestWriterFactory, namespace, name string) bool {
		events := readerFactory.Reader.Events

		// Setup router.
		router := chi.NewRouter()
		router.Use(func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := r.Context()
				ctx = context.WithValue(ctx, ContextReaderFactory, &readerFactory)
				ctx = context.WithValue(ctx, ContextWriterFactory, &writerFactory)
				r = r.WithContext(ctx)
				h.ServeHTTP(w, r)
			})
		})
		router.Get("/{namespace}/{name}", withLogger(watchWS, zap.NewNop().Sugar()))

		// Setup test server.
		ts := httptest.NewServer(router)
		defer ts.Close()

		ts.Config.BaseContext = func(listener net.Listener) context.Context {
			ctx := context.Background()
			ctx = context.WithValue(ctx, ContextReaderFactory, readerFactory)
			ctx = context.WithValue(ctx, ContextWriterFactory, writerFactory)
			return ctx
		}

		// Make a request.
		requestPath := fmt.Sprintf("/%s/%s", namespace, name)
		resp, err := http.Get(fmt.Sprintf("%s%s", ts.URL, requestPath))
		if err != nil {
			t.Errorf("request failed: %s", err)
			return false
		}

		// Read response.
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Error("couldn't read response body")
			return false
		}
		message := string(body)

		// Check response.
		if resp.StatusCode == http.StatusOK {
			if readWriteError(readerFactory, writerFactory) && len(readerFactory.Reader.Events) > 0 {
				t.Error("200 returned when read or write error occurred")
				return false
			} else if namespace == "" || name == "" {
				t.Errorf("200 returned when namespace or name is empty: '%s'/'%s'", namespace, name)
				return false
			} else if readerFactory.Namespace != namespace || readerFactory.Name != name {
				t.Errorf("namespace or name doesn't match")
				return false
			}
		} else if resp.StatusCode == http.StatusBadRequest {
			if name == "" || namespace == "" {
				t.Log("namespace or name is empty")
				return true
			} else {
				t.Errorf("bad request: %s", message)
				return false
			}
		} else if resp.StatusCode == http.StatusInternalServerError {
			if readWriteError(readerFactory, writerFactory) {
				t.Log("error occurred during reading or writing")
				return true
			} else if closeError(readerFactory, writerFactory) {
				t.Log("error occurred during closing of reader or writer")
			} else {
				t.Errorf("500 returned when no error was returned: %s", message)
				return false
			}
		} else {
			t.Errorf("handler returned invalid response code %d", resp.StatusCode)
			return false
		}

		// Check whether all events were passed from reader to writer.
		eventsSent := writerFactory.Writer.Events
		if !reflect.DeepEqual(events, eventsSent) && !EOFError(readerFactory, writerFactory) {
			t.Errorf("events were not passed correctly: %d read, %d sent", len(events), len(eventsSent))
			return false
		}

		t.Logf("succeeded: %d events", len(eventsSent))
		return true
	}

	if err := quick.Check(f, &quick.Config{Rand: r}); err != nil {
		t.Fatal(err)
	}
}
