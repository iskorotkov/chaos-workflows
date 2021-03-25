package test

import (
	"fmt"
	"github.com/gorilla/websocket"
	http_helper "github.com/gruntwork-io/terratest/modules/http-helper"
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

	t.Run("docker image can be built", func(t *testing.T) {
		t.Logf("Building image: %s", image)
		shell.RunCommand(t, shell.Command{
			WorkingDir: "../.",
			Command:    "docker",
			Args:       []string{"build", "-f", "./build/workflows.dockerfile", "-t", image, "."},
		})
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
		t.Fatal(err)
	}

	var builder strings.Builder
	if err := tpl.Execute(&builder, struct {
		Image     string
		Namespace string
	}{image, namespace}); err != nil {
		t.Fatal(err)
	}

	t.Logf("Creating namespace: %s", namespace)
	k8s.CreateNamespace(t, defaultOptions, namespace)
	defer k8s.DeleteNamespace(t, defaultOptions, namespace)

	t.Logf("Deploying to Kubernetes")
	k8s.KubectlApplyFromString(t, serviceOptions, builder.String())
	defer k8s.KubectlDeleteFromString(t, serviceOptions, builder.String())

	t.Run("all pods are created", func(t *testing.T) {
		k8s.WaitUntilNumPodsCreated(t, serviceOptions, v1.ListOptions{}, 1, 10, time.Second)
	})

	t.Run("service is available", func(t *testing.T) {
		k8s.WaitUntilServiceAvailable(t, serviceOptions, "workflows", 10, time.Second)
	})

	service := k8s.GetService(t, serviceOptions, "workflows")
	endpoint := k8s.GetServiceEndpoint(t, serviceOptions, service, 8811)

	//goland:noinspection HttpUrlsUsage
	url := fmt.Sprintf("http://%s/api/v1/workflows/watch/litmus/this-workflow-does-not-exist", endpoint)

	t.Run("http connection fails because it can't switch to websocket connection", func(t *testing.T) {
		http_helper.HttpGetWithRetry(t, url, nil, 400, "handshake error: bad \"Upgrade\" header", 10, time.Second)
	})

	t.Run("websocket connection fails when workflow doesn't exist", func(t *testing.T) {
		wsConnection(t, url)
	})
}

func TestDeployed(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped")
	}

	options := k8s.NewKubectlOptions("", "", "chaos-framework")
	service := k8s.GetService(t, options, "workflows")
	endpoint := k8s.GetServiceEndpoint(t, options, service, 8811)
	url := fmt.Sprintf("ws://%s/api/v1/workflows/watch/litmus/this-workflow-does-not-exist", endpoint)

	t.Run("websocket connection fails when workflow doesn't exist", func(t *testing.T) {
		wsConnection(t, url)
	})
}

func wsConnection(t *testing.T, url string) {
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}

	defer func(conn *websocket.Conn) {
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}(conn)

	_ = resp

	m := make(map[string]interface{})
	if err := conn.ReadJSON(m); err != nil {
		t.Fatal(err)
	}
}
