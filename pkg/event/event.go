package event

import (
	"context"
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
	ErrSpecAnalysisFailed = errors.New("couldn't find step spec")
)

type Watcher interface {
	Watch(ctx context.Context, namespace, name string) (Reader, error)
	Close() error
}

type Reader interface {
	Read() (Event, error)
	Close() error
}

type Writer interface {
	Write(ctx context.Context, ev Event) error
	Close() error
}

type Step struct {
	Name        string            `json:"name,omitempty"`
	Type        string            `json:"type,omitempty"`
	Phase       string            `json:"phase,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	StartedAt   time.Time         `json:"startedAt,omitempty"`
	FinishedAt  time.Time         `json:"finishedAt,omitempty"`
}

type Stage struct {
	Phase      string    `json:"phase,omitempty"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	Steps      []Step    `json:"steps,omitempty"`
}

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

func buildNodesTree(ts []v1alpha1.Template, nodes nodes) ([]Stage, error) {
	stagesIDs, stepsIDs := splitStagesAndSteps(nodes)

	stages := make([]Stage, 0)
	for i := 0; i < len(stagesIDs); i++ {
		id := fmt.Sprintf("[%d]", i)
		stageStatus := stagesIDs[id]
		steps := make([]Step, 0)
		for _, stepID := range stageStatus.Children {
			stepStatus := stepsIDs[stepID]
			stepSpec, err := findStepSpec(ts, stepStatus)
			if err != nil {
				return nil, err
			}

			steps = append(steps, newStep(stepSpec.Metadata, stepStatus))
		}

		stages = append(stages, newStage(stageStatus, steps))
	}

	return stages, nil
}

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

func findStepSpec(ts []v1alpha1.Template, stepStatus v1alpha1.NodeStatus) (v1alpha1.Template, error) {
	for _, t := range ts {
		if t.Name == stepStatus.TemplateName {
			return t, nil
		}
	}

	return v1alpha1.Template{}, ErrSpecAnalysisFailed
}

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

func newStage(n v1alpha1.NodeStatus, steps []Step) Stage {
	return Stage{
		Phase:      string(n.Phase),
		StartedAt:  n.StartedAt.Time,
		FinishedAt: n.FinishedAt.Time,
		Steps:      steps,
	}
}

func NewEvent(e *workflow.WorkflowWatchEvent) (Event, error) {
	stages, err := buildNodesTree(e.Object.Spec.Templates, nodes(e.Object.Status.Nodes))
	if err != nil {
		return Event{}, err
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
	}, nil
}
