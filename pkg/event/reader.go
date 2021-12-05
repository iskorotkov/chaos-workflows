package event

import (
	"math/rand"
	"reflect"
)

// Reader reads a sequence of workflow events for a specific workflow.
type Reader interface {
	Read() (Workflow, error)
	Close() error
}

// TestReader mocks Reader interface.
type TestReader struct {
	Events  []Workflow
	ErrRead error

	Closed   bool
	ErrClose error
}

func (t TestReader) Generate(rand *rand.Rand, size int) reflect.Value {
	var events []Workflow
	for i := 0; i < 1+rand.Intn(10); i++ {
		events = append(events, Workflow{}.Generate(rand, size).Interface().(Workflow))
	}

	return reflect.ValueOf(TestReader{
		Events:   events,
		ErrRead:  GenerateTestError(rand),
		ErrClose: GenerateTestError(rand),
	})
}

func (t *TestReader) Read() (Workflow, error) {
	if t.ErrRead != nil {
		return Workflow{}, t.ErrRead
	}

	e := t.Events[0]
	t.Events = t.Events[1:]

	if len(t.Events) == 0 {
		return e, ErrAllRead
	}

	return e, nil
}

func (t *TestReader) Close() error {
	t.Closed = true
	return t.ErrClose
}
