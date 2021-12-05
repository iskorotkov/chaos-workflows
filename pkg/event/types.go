// Package event handles conversion from Argo/Kubernetes event format to a custom one.
package event

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"time"

	"github.com/argoproj/argo-workflows/v3/pkg/apiclient/workflow"
	"github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	typeKey     = "chaosframework.com/type"
	severityKey = "chaosframework.com/severity"
	scaleKey    = "chaosframework.com/scale"
	versionKey  = "chaosframework.com/version"
)

var (
	ErrAllRead          = errors.New("no more events available")
	ErrDeadlineExceeded = errors.New("streaming was finished due to a timeout")
	ErrInvalidEvent     = errors.New("event was in invalid format")
	ErrConnectionFailed = errors.New("couldn't establish connection to external service")
)

// GenerateTestError returns a random error from package.
func GenerateTestError(rand *rand.Rand) error {
	eventErrors := map[error]float32{
		ErrAllRead:          0.05,
		ErrDeadlineExceeded: 0.05,
		ErrInvalidEvent:     0.05,
		ErrConnectionFailed: 0.05,
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
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	Severity   string     `json:"severity"`
	Scale      string     `json:"scale"`
	Status     string     `json:"status"`
	Version    string     `json:"version"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

func (s Step) Generate(rand *rand.Rand, _ int) reflect.Value {
	f := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, rand.Intn(10))
	}
	finishedAt := time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Minute)
	return reflect.ValueOf(Step{
		Name:       f("name"),
		Type:       f("type"),
		Severity:   f("severity"),
		Scale:      f("scale"),
		Status:     f("status"),
		Version:    f("version"),
		StartedAt:  time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Hour),
		FinishedAt: &finishedAt,
	})
}

// Stage is a part of a workflow.
type Stage struct {
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	Steps      []Step     `json:"steps"`
}

func (s Stage) Generate(rand *rand.Rand, size int) reflect.Value {
	f := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, rand.Intn(10))
	}

	var steps []Step
	for i := 0; i < rand.Intn(10); i++ {
		steps = append(steps, Step{}.Generate(rand, size).Interface().(Step))
	}

	finishedAt := time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Minute)
	return reflect.ValueOf(Stage{
		Status:     f("phase"),
		StartedAt:  time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Hour),
		FinishedAt: &finishedAt,
		Steps:      steps,
	})
}

// Workflow is a workflow update message.
type Workflow struct {
	Name       string     `json:"name"`
	Namespace  string     `json:"namespace"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	// Type is a type of event (when listening to workflow events).
	Type   string  `json:"type,omitempty"`
	Status string  `json:"status"`
	Stages []Stage `json:"stages"`
}

func (e Workflow) Generate(rand *rand.Rand, size int) reflect.Value {
	f := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, rand.Intn(10))
	}

	var stages []Stage
	for i := 0; i < rand.Intn(10); i++ {
		stages = append(stages, Stage{}.Generate(rand, size).Interface().(Stage))
	}

	finishedAt := time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Minute)
	return reflect.ValueOf(Workflow{
		Name:       f("name"),
		Namespace:  f("namespace"),
		Type:       f("type"),
		Status:     f("status"),
		StartedAt:  time.Time{}.Add(-time.Duration(rand.Intn(10)) * time.Hour),
		FinishedAt: &finishedAt,
		Stages:     stages,
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

func FromWorkflow(w v1alpha1.Workflow) (Workflow, bool) {
	stages, ok := buildNodesTree(w.Spec.Templates, nodes(w.Status.Nodes))
	if !ok {
		return Workflow{}, false
	}

	finishedAt := &w.Status.FinishedAt.Time
	if *finishedAt == (time.Time{}) {
		finishedAt = nil
	}

	return Workflow{
		Name:       w.Name,
		Namespace:  w.Namespace,
		Type:       "", // No type set for non-event value.
		Status:     strings.ToLower(string(w.Status.Phase)),
		StartedAt:  w.Status.StartedAt.Time,
		FinishedAt: finishedAt,
		Stages:     stages,
	}, true
}

func FromWorkflowEvent(e *workflow.WorkflowWatchEvent) (Workflow, bool) {
	stages, ok := buildNodesTree(e.Object.Spec.Templates, nodes(e.Object.Status.Nodes))
	if !ok {
		return Workflow{}, false
	}

	finishedAt := &e.Object.Status.FinishedAt.Time
	if *finishedAt == (time.Time{}) {
		finishedAt = nil
	}

	return Workflow{
		Name:       e.Object.Name,
		Namespace:  e.Object.Namespace,
		Type:       e.Type,
		Status:     strings.ToLower(string(e.Object.Status.Phase)),
		StartedAt:  e.Object.Status.StartedAt.Time,
		FinishedAt: finishedAt,
		Stages:     stages,
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
	finishedAt := &n.FinishedAt.Time
	if *finishedAt == (time.Time{}) {
		finishedAt = nil
	}

	return Step{
		Name:       n.TemplateName,
		Type:       metadata.Annotations[typeKey],
		Severity:   metadata.Annotations[severityKey],
		Scale:      metadata.Annotations[scaleKey],
		Version:    metadata.Annotations[versionKey],
		Status:     strings.ToLower(string(n.Phase)),
		StartedAt:  n.StartedAt.Time,
		FinishedAt: finishedAt,
	}
}

// newStage returns new Stage from v1alpha1.NodeStatus and list of Step.
func newStage(n v1alpha1.NodeStatus, steps []Step) Stage {
	finishedAt := &n.FinishedAt.Time
	if *finishedAt == (time.Time{}) {
		finishedAt = nil
	}

	return Stage{
		Status:     strings.ToLower(string(n.Phase)),
		StartedAt:  n.StartedAt.Time,
		FinishedAt: finishedAt,
		Steps:      steps,
	}
}
