package event

import (
	"context"
	"math/rand"
	"reflect"
)

// Writer writes a sequence of workflow events for a specific workflow.
type Writer interface {
	Write(ctx context.Context, ev Workflow) error
	Close() error
}

// TestWriter mocks Writer interface.
type TestWriter struct {
	Events   []Workflow
	ErrWrite error

	Closed   bool
	ErrClose error
}

func (t TestWriter) Generate(rand *rand.Rand, _ int) reflect.Value {
	return reflect.ValueOf(TestWriter{
		ErrWrite: GenerateTestError(rand),
		ErrClose: GenerateTestError(rand),
	})
}

func (t *TestWriter) Write(_ context.Context, ev Workflow) error {
	t.Events = append(t.Events, ev)
	return t.ErrWrite
}

func (t *TestWriter) Close() error {
	t.Closed = true
	return t.ErrClose
}
