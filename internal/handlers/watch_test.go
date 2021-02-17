package handlers

import (
	"context"
	"fmt"
	"github.com/go-chi/chi"
	"github.com/iskorotkov/chaos-workflows/pkg/event"
	"go.uber.org/zap"
	"math/rand"
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

	readWriteError := func(readerFactory TestReaderFactory, writerFactory TestWriterFactory) bool {
		return readerFactory.ErrNew != nil ||
			writerFactory.ErrNew != nil ||
			readerFactory.Reader.ErrRead != nil && readerFactory.Reader.ErrRead != event.ErrLastEvent ||
			writerFactory.Writer.ErrWrite != nil && writerFactory.Writer.ErrWrite != event.ErrLastEvent
	}
	_ = readWriteError

	f := func(readerFactory TestReaderFactory, writerFactory TestWriterFactory, namespace, name string) bool {
		events := readerFactory.Reader.Events

		// Setup router.
		router := chi.NewRouter()
		router.Get("/{namespace}/{name}", func(writer http.ResponseWriter, request *http.Request) {
			watchWS(writer, request, &readerFactory, &writerFactory, zap.NewNop().Sugar())
		})

		// Setup test server.
		ts := httptest.NewServer(router)
		defer ts.Close()

		// Make a request.
		requestPath := fmt.Sprintf("/%s/%s", namespace, name)
		resp, err := http.Get(fmt.Sprintf("%s%s", ts.URL, requestPath))
		if err != nil {
			t.Errorf("request failed: %s", err)
			return false
		}

		eventsSent := writerFactory.Writer.Events

		// If input parameters are invalid.
		if name == "" || namespace == "" {
			if len(eventsSent) == 0 && resp.StatusCode == http.StatusNotFound {
				t.Log("name or namespace was empty")
				return true
			} else {
				t.Log("name or namespace was empty but handler didn't return correct error")
				return false
			}
		}

		// Check if all params were passed correctly.
		if readerFactory.Name != name || readerFactory.Namespace != namespace {
			t.Log("name or namespace was not passed to reader factory")
			return false
		}

		// If reader/writer creation failed.
		if readerFactory.ErrNew != nil || writerFactory.ErrNew != nil {
			if len(eventsSent) == 0 && resp.StatusCode == http.StatusInternalServerError {
				t.Logf("reader/writer creation failed")
				return true
			} else {
				t.Logf("reader/writer creation failed but handler didn't return correct error")
				return false
			}
		}

		// If reading failed.
		if readerFactory.Reader.ErrRead != nil &&
			readerFactory.Reader.ErrRead != event.ErrLastEvent {
			if len(eventsSent) == 0 {
				t.Logf("reading failed")
				return true
			} else {
				t.Logf("reading failed but handler didn't return correct error")
				return false
			}
		}

		// If writing failed.
		if writerFactory.Writer.ErrWrite != nil {
			if len(eventsSent) == 1 {
				t.Logf("writing failed")
				return true
			} else {
				t.Logf("writing failed but handler didn't return correct error")
				return false
			}
		}

		// Check response.
		if resp.StatusCode != http.StatusOK {
			t.Errorf("returned status code: %d", resp.StatusCode)
			return false
		}

		// If sent extra events.
		if len(eventsSent) > len(events) {
			t.Errorf("sent more events that was read: %d > %d", len(eventsSent), len(events))
			return false
		}

		// If dropped some events.
		if !reflect.DeepEqual(events, eventsSent) && readerFactory.Reader.ErrRead != event.ErrLastEvent {
			t.Errorf("events were not passed correctly: %d read, %d sent", len(events), len(eventsSent))
			return false
		}

		// If reader or writer wasn't closed.
		if !readerFactory.Reader.Closed || !writerFactory.Writer.Closed {
			t.Error("reader or writer wasn't closed")
			return false
		}

		t.Logf("succeeded: %d/%d events", len(eventsSent), len(events))
		return true
	}

	if err := quick.Check(f, &quick.Config{Rand: r}); err != nil {
		t.Fatal(err)
	}
}
