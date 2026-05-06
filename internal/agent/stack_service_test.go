package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiv1 "github.com/ingresslabs/torque/pkg/api/torque/api/v1"
)

func TestStackServicePlanCompilesStackOnAgentHost(t *testing.T) {
	root := t.TempDir()
	stackYAML := `
name: remote-agent-stack
cli:
  inferDeps: false
releases:
  - name: api
    chart: ./chart
    cluster: {name: c1}
    namespace: default
`
	if err := os.WriteFile(filepath.Join(root, "stack.yaml"), []byte(strings.TrimSpace(stackYAML)+"\n"), 0o644); err != nil {
		t.Fatalf("write stack.yaml: %v", err)
	}

	resp, err := (&StackServer{}).Plan(context.Background(), &apiv1.StackPlanRequest{
		Selector: &apiv1.StackSelector{Root: root},
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if resp.GetStackName() != "remote-agent-stack" || resp.GetNodeCount() != 1 {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if !strings.Contains(resp.GetJson(), `"name": "api"`) {
		t.Fatalf("plan json did not include release: %s", resp.GetJson())
	}
}
