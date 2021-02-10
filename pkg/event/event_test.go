package event

import (
	"errors"
	"github.com/argoproj/argo/pkg/apis/workflow/v1alpha1"
	"math/rand"
	"testing"
	"testing/quick"
	"time"
)

func validateTime(start time.Time, finish time.Time) error {
	if !start.Before(finish) {
		return errors.New("start time must be before finish time")
	}

	if finish.Sub(start) > 2*time.Hour {
		return errors.New("workflow must be executed for 2 hours at most")
	}

	return nil
}

func Test_buildNodesTree(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(0))
	f := func(ts []v1alpha1.Template, nodes nodes) bool {
		stages, err := buildNodesTree(ts, nodes)
		if err != nil {
			t.Log(err)
			return false
		}

		for _, stage := range stages {
			if stage.Phase == "" {
				t.Log("stage phase must not be empty")
				return false
			}

			if err := validateTime(stage.StartedAt, stage.FinishedAt); err != nil {
				t.Log(err)
				return false
			}

			for _, step := range stage.Steps {
				if err := validateTime(step.StartedAt, step.FinishedAt); err != nil {
					t.Log(err)
					return false
				}

				if step.Name == "" ||
					step.Phase == "" ||
					step.Type == "" {
					t.Log("step name, phase and type must not be empty")
					return false
				}
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{Rand: r}); err != nil {
		t.Error(err)
	}
}
