package mcpserver

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMCPDAGBuildScenarioCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "mcp", "dag-build-scenarios.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scenario catalog: %v", err)
	}

	var catalog mcpScenarioCatalog
	if err := yaml.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("parse scenario catalog: %v", err)
	}
	if catalog.Version != "torque.mcp.scenarios/v1" {
		t.Fatalf("unexpected catalog version %q", catalog.Version)
	}
	if len(catalog.Scenarios) < 10 || len(catalog.Scenarios) > 20 {
		t.Fatalf("scenario count = %d, want 10..20", len(catalog.Scenarios))
	}

	ids := map[string]bool{}
	calls := map[string]bool{}
	remoteServices := map[string]bool{}
	redactions := map[string]bool{}
	writeRequirements := map[string]bool{}
	for _, scenario := range catalog.Scenarios {
		if scenario.ID == "" {
			t.Fatalf("scenario with empty id: %+v", scenario)
		}
		if ids[scenario.ID] {
			t.Fatalf("duplicate scenario id %q", scenario.ID)
		}
		ids[scenario.ID] = true
		if scenario.Title == "" {
			t.Fatalf("scenario %q has empty title", scenario.ID)
		}
		if len(scenario.Topology.Nodes) < 3 {
			t.Fatalf("scenario %q has too few nodes: %v", scenario.ID, scenario.Topology.Nodes)
		}
		if len(scenario.Topology.Edges) < 2 {
			t.Fatalf("scenario %q has too few edges: %v", scenario.ID, scenario.Topology.Edges)
		}
		if len(scenario.MCP.Calls) == 0 {
			t.Fatalf("scenario %q has no MCP calls", scenario.ID)
		}
		if len(scenario.Validation.Expect) == 0 {
			t.Fatalf("scenario %q has no validation expectations", scenario.ID)
		}
		assertScenarioDAG(t, scenario)

		for _, call := range scenario.MCP.Calls {
			calls[call] = true
		}
		for _, service := range scenario.MCP.RemoteGRPC {
			remoteServices[service] = true
		}
		for _, value := range scenario.Safety.Redactions {
			redactions[value] = true
		}
		for _, value := range scenario.Safety.WriteRequires {
			writeRequirements[value] = true
		}
	}

	for _, required := range []string{
		"torque.build.run",
		"torque.cache.inspect",
		"torque.cache.plan",
		"torque.cache.warm",
		"torque.ship.run",
		"torque.stack.apply",
		"torque.stack.delete",
		"torque.logs.query",
	} {
		if !calls[required] {
			t.Fatalf("scenario catalog does not cover MCP call %q", required)
		}
	}
	for _, required := range []string{
		"BuildService.RunBuild",
		"DeployService.Apply",
		"StackService.Apply",
		"MirrorService.Subscribe",
	} {
		if !remoteServices[required] {
			t.Fatalf("scenario catalog does not cover remote gRPC service %q", required)
		}
	}
	if !redactions["TORQUE_REMOTE_TOKEN"] && !redactions["remote-token"] {
		t.Fatalf("scenario catalog does not cover remote token redaction")
	}
	if !writeRequirements["safety.confirm"] {
		t.Fatalf("scenario catalog does not cover safety.confirm")
	}
}

func assertScenarioDAG(t *testing.T, scenario mcpScenario) {
	t.Helper()
	nodes := map[string]bool{}
	for _, node := range scenario.Topology.Nodes {
		if node == "" {
			t.Fatalf("scenario %q has empty node", scenario.ID)
		}
		nodes[node] = true
	}

	adjacency := map[string][]string{}
	for _, edge := range scenario.Topology.Edges {
		if len(edge) != 2 {
			t.Fatalf("scenario %q edge %v does not contain two endpoints", scenario.ID, edge)
		}
		from, to := edge[0], edge[1]
		if !nodes[from] {
			t.Fatalf("scenario %q edge starts at unknown node %q", scenario.ID, from)
		}
		if !nodes[to] {
			t.Fatalf("scenario %q edge ends at unknown node %q", scenario.ID, to)
		}
		adjacency[from] = append(adjacency[from], to)
	}

	state := map[string]int{}
	var visit func(string) bool
	visit = func(node string) bool {
		switch state[node] {
		case 1:
			return false
		case 2:
			return true
		}
		state[node] = 1
		for _, next := range adjacency[node] {
			if !visit(next) {
				return false
			}
		}
		state[node] = 2
		return true
	}
	for _, node := range scenario.Topology.Nodes {
		if !visit(node) {
			t.Fatalf("scenario %q topology is cyclic at %q", scenario.ID, node)
		}
	}
}

type mcpScenarioCatalog struct {
	Version     string        `yaml:"version"`
	Scenarios   []mcpScenario `yaml:"scenarios"`
	Description string        `yaml:"description"`
}

type mcpScenario struct {
	ID       string `yaml:"id"`
	Title    string `yaml:"title"`
	Topology struct {
		Shape string     `yaml:"shape"`
		Nodes []string   `yaml:"nodes"`
		Edges [][]string `yaml:"edges"`
	} `yaml:"topology"`
	Build struct {
		Mode      string   `yaml:"mode"`
		Contexts  []string `yaml:"contexts"`
		Artifacts []string `yaml:"artifacts"`
		Services  []string `yaml:"services"`
		Cache     []string `yaml:"cache"`
		Secrets   []string `yaml:"secrets"`
	} `yaml:"build"`
	MCP struct {
		Calls      []string `yaml:"calls"`
		RemoteGRPC []string `yaml:"remoteGrpc"`
	} `yaml:"mcp"`
	Safety struct {
		WriteRequires []string `yaml:"writeRequires"`
		Redactions    []string `yaml:"redactions"`
	} `yaml:"safety"`
	Validation struct {
		Expect []string `yaml:"expect"`
	} `yaml:"validation"`
}
