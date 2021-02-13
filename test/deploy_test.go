package test

import (
	"fmt"
	"github.com/gruntwork-io/terratest/modules/http-helper"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/shell"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path"
	"strings"
	"testing"
	"text/template"
	"time"
)

func TestDeploy(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped")
	}

	timestamp := time.Now().Format("20060102-150405")
	image := fmt.Sprintf("iskorotkov/chaos-workflows:test-%s", timestamp)
	namespace := fmt.Sprintf("test-workflows-%s", timestamp)

	defaultOptions := k8s.NewKubectlOptions("", "", "default")
	serviceOptions := k8s.NewKubectlOptions("", "", namespace)

	t.Logf("Building image: %s", image)
	shell.RunCommand(t, shell.Command{
		WorkingDir: "../.",
		Command:    "docker",
		Args:       []string{"build", "-f", "./build/workflows.dockerfile", "-t", image, "."},
	})

	t.Logf("Pushing image: %s", image)
	shell.RunCommand(t, shell.Command{
		Command: "docker",
		Args:    []string{"push", image},
	})

	// Assume that Litmus is available in litmus namespace
	// Assume that Argo is available in argo namespace

	t.Logf("Preparing YAML manifest file")
	tpl, err := template.ParseFiles(path.Join("../", "./deploy/workflows-test.yaml"))
	if err != nil {
		panic(err)
	}

	var builder strings.Builder
	if err := tpl.Execute(&builder, struct {
		Image     string
		Namespace string
	}{image, namespace}); err != nil {
		panic(err)
	}

	t.Logf("Creating namespace: %s", namespace)
	k8s.CreateNamespace(t, defaultOptions, namespace)
	defer k8s.DeleteNamespace(t, defaultOptions, namespace)

	t.Logf("Deploying to Kubernetes")
	k8s.KubectlApplyFromString(t, serviceOptions, builder.String())
	defer k8s.KubectlDeleteFromString(t, serviceOptions, builder.String())

	k8s.WaitUntilNumPodsCreated(t, serviceOptions, v1.ListOptions{}, 1, 10, time.Second)

	k8s.WaitUntilServiceAvailable(t, serviceOptions, "workflows", 10, time.Second)

	service := k8s.GetService(t, serviceOptions, "workflows")
	endpoint := k8s.GetServiceEndpoint(t, serviceOptions, service, 8811)

	//goland:noinspection HttpUrlsUsage
	url := fmt.Sprintf("http://%s/api/v1/workflows/litmus/this-workflow-does-not-exist", endpoint)

	// Returns error because it can't switch to websocket connection.
	http_helper.HttpGetWithRetry(t, url, nil, 400, "handshake error: bad \"Upgrade\" header", 10, time.Second)
}
