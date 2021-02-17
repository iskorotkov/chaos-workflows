// Package event handles conversion from Argo/Kubernetes event format to a custom one.
package event

import (
	"errors"
	"fmt"
	"github.com/argoproj/argo/pkg/apiclient/workflow"
	"github.com/argoproj/argo/pkg/apis/workflow/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"math/rand"
	"reflect"
	"time"
)

var (
	ErrLastEvent       = errors.New("no more events available")
	ErrTimeout         = errors.New("streaming was finished due to a timeout")
	ErrInvalidEvent    = errors.New("event was in invalid format")
	ErrInternalFailure = errors.New("internal error occurred")
	ErrAlreadyClosed   = errors.New("connection was already closed")
)

// GenerateTestError returns a random error from package.
func GenerateTestError(rand *rand.Rand) error {
	eventErrors := map[error]float32{
		ErrLastEvent:       0.05,
		ErrTimeout:         0.05,
		ErrInvalidEvent:    0.05,
		ErrInternalFailure: 0.05,
		ErrAlreadyClosed:   0.05,
	}

	value := rand.Float32()
	for k, v := range eventErrors {
		if v > value {
			return k
		}

		value -= v
	}

	return nil
}

// Step is a part of a stage.
type Step struct {
	Name        string            `json:"name,omitempty"`
	Type        string            `json:"type,omitempty"`
	Phase       string            `json:"phase,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	StartedAt   time.Time         `json:"startedAt,omitempty"`
	FinishedAt  time.Time         `json:"finishedAt,omitempty"`
}

func (s Step) Generate(rand *rand.Rand, _ int) reflect.Value {
	f := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, rand.Intn(10))
	}
	return reflect.ValueOf(Step{
		Name:        f("name"),
		Type:        f("type"),
		Phase:       f("phase"),
		Labels:      map[string]string{"label1": "value1", "label2": "value2"},
		Annotations: map[string]string{"ann1": "value3", "ann2": "value4"},
		StartedAt:   time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Hour),
		FinishedAt:  time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Minute),
	})
}

// Stage is a part of a workflow.
type Stage struct {
	Phase      string    `json:"phase,omitempty"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	Steps      []Step    `json:"steps,omitempty"`
}

func (s Stage) Generate(rand *rand.Rand, size int) reflect.Value {
	f := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, rand.Intn(10))
	}

	var steps []Step
	for i := 0; i < rand.Intn(10); i++ {
		steps = append(steps, Step{}.Generate(rand, size).Interface().(Step))
	}

	return reflect.ValueOf(Stage{
		Phase:      f("phase"),
		StartedAt:  time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Hour),
		FinishedAt: time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Minute),
		Steps:      steps,
	})
}

// Event is a workflow update message.
type Event struct {
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Type        string            `json:"type,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Phase       string            `json:"phase,omitempty"`
	StartedAt   time.Time         `json:"startedAt,omitempty"`
	FinishedAt  time.Time         `json:"finishedAt,omitempty"`
	Stages      []Stage           `json:"stages,omitempty"`
}

func (e Event) Generate(rand *rand.Rand, size int) reflect.Value {
	f := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, rand.Intn(10))
	}

	var stages []Stage
	for i := 0; i < rand.Intn(10); i++ {
		stages = append(stages, Stage{}.Generate(rand, size).Interface().(Stage))
	}

	return reflect.ValueOf(Event{
		Name:        f("name"),
		Namespace:   f("namespace"),
		Type:        f("type"),
		Labels:      map[string]string{"label1": "value1", "label2": "value2"},
		Annotations: map[string]string{"ann1": "value3", "ann2": "value4"},
		Phase:       f("phase"),
		StartedAt:   time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Hour),
		FinishedAt:  time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Minute),
		Stages:      stages,
	})
}

type nodes v1alpha1.Nodes

func (n nodes) Generate(r *rand.Rand, _ int) reflect.Value {
	rs := func(s string) string {
		return fmt.Sprintf("%s-%d", s, r.Int())
	}

	nodeTypes := []v1alpha1.NodeType{v1alpha1.NodeTypeDAG, v1alpha1.NodeTypeSteps, v1alpha1.NodeTypeSuspend}
	nodePhases := []v1alpha1.NodePhase{v1alpha1.NodeSucceeded, v1alpha1.NodeFailed, v1alpha1.NodePending, v1alpha1.NodeRunning}

	statuses := make(nodes)
	for i := 0; i < r.Intn(10); i++ {
		statuses[rs("name")] = v1alpha1.NodeStatus{
			ID:            rs("id"),
			Name:          rs("name"),
			DisplayName:   rs("display-name"),
			Type:          nodeTypes[r.Intn(len(nodeTypes))],
			TemplateName:  rs("template-name"),
			TemplateScope: rs("template-scope"),
			Phase:         nodePhases[r.Intn(len(nodePhases))],
			BoundaryID:    rs("boundary-id"),
			Message:       rs("message"),
			StartedAt:     v1.Time{Time: time.Now().Add(-5 * time.Hour)},
			FinishedAt:    v1.Time{Time: time.Now().Add(-20 * time.Minute)},
			PodIP:         rs("pod-ip"),
		}
	}

	return reflect.ValueOf(statuses)
}

func ToCustomEvent(e *workflow.WorkflowWatchEvent) (Event, bool) {
	stages, ok := buildNodesTree(e.Object.Spec.Templates, nodes(e.Object.Status.Nodes))
	if !ok {
		return Event{}, false
	}

	return Event{
		Name:        e.Object.Name,
		Namespace:   e.Object.Namespace,
		Type:        e.Type,
		Labels:      e.Object.Labels,
		Annotations: e.Object.Annotations,
		Phase:       string(e.Object.Status.Phase),
		StartedAt:   e.Object.Status.StartedAt.Time,
		FinishedAt:  e.Object.Status.FinishedAt.Time,
		Stages:      stages,
	}, true
}

// buildNodesTree parses spec and status to build a hierarchy of stages and steps.
func buildNodesTree(ts []v1alpha1.Template, nodes nodes) ([]Stage, bool) {
	stagesIDs, stepsIDs := splitStagesAndSteps(nodes)

	stages := make([]Stage, 0)

	// For each stage.
	for i := 0; i < len(stagesIDs); i++ {
		id := fmt.Sprintf("[%d]", i)
		stageStatus := stagesIDs[id]

		steps := make([]Step, 0)

		// For each step.
		for _, stepID := range stageStatus.Children {
			stepStatus := stepsIDs[stepID]
			stepSpec, ok := findStepSpec(stepStatus, ts)
			if !ok {
				return nil, false
			}

			steps = append(steps, newStep(stepSpec.Metadata, stepStatus))
		}

		stages = append(stages, newStage(stageStatus, steps))
	}

	return stages, true
}

// splitStagesAndSteps separates stages and steps into two maps.
func splitStagesAndSteps(nodes nodes) (map[string]v1alpha1.NodeStatus, map[string]v1alpha1.NodeStatus) {
	stagesID := make(map[string]v1alpha1.NodeStatus)
	stepsID := make(map[string]v1alpha1.NodeStatus)

	for _, n := range nodes {
		if n.Type == "StepGroup" {
			stagesID[n.DisplayName] = n
		} else {
			stepsID[n.ID] = n
		}
	}
	return stagesID, stepsID
}

// findStepSpec returns Step's spec given its v1alpha1.NodeStatus.
func findStepSpec(stepStatus v1alpha1.NodeStatus, ts []v1alpha1.Template) (v1alpha1.Template, bool) {
	for _, t := range ts {
		if t.Name == stepStatus.TemplateName {
			return t, true
		}
	}

	return v1alpha1.Template{}, false
}

// newStep returns new Step from v1alpha1.Metadata and v1alpha1.NodeStatus.
func newStep(metadata v1alpha1.Metadata, n v1alpha1.NodeStatus) Step {
	return Step{
		Name:        n.TemplateName,
		Type:        string(n.Type),
		Phase:       string(n.Phase),
		Labels:      metadata.Labels,
		Annotations: metadata.Annotations,
		StartedAt:   n.StartedAt.Time,
		FinishedAt:  n.FinishedAt.Time,
	}
}

// newStage returns new Stage from v1alpha1.NodeStatus and list of Step.
func newStage(n v1alpha1.NodeStatus, steps []Step) Stage {
	return Stage{
		Phase:      string(n.Phase),
		StartedAt:  n.StartedAt.Time,
		FinishedAt: n.FinishedAt.Time,
		Steps:      steps,
	}
}
