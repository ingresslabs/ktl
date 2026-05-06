package mcpserver

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	apiv1 "github.com/ingresslabs/torque/pkg/api/torque/api/v1"
	"google.golang.org/grpc"
)

type fakeTorqueAgent struct {
	apiv1.UnimplementedAgentInfoServiceServer
	apiv1.UnimplementedMirrorServiceServer
	apiv1.UnimplementedLogServiceServer
	apiv1.UnimplementedBuildServiceServer
	apiv1.UnimplementedDeployServiceServer
	apiv1.UnimplementedVerifyServiceServer
	mu     sync.Mutex
	builds []*apiv1.BuildOptions
}

type fakeStackAgent struct {
	apiv1.UnimplementedStackServiceServer
}

func (f *fakeTorqueAgent) GetInfo(context.Context, *apiv1.AgentInfoRequest) (*apiv1.AgentInfo, error) {
	return &apiv1.AgentInfo{Version: "remote-test", GitCommit: "abc123", Platform: "linux/amd64"}, nil
}

func (f *fakeTorqueAgent) ListSessions(context.Context, *apiv1.MirrorListSessionsRequest) (*apiv1.MirrorListSessionsResponse, error) {
	return &apiv1.MirrorListSessionsResponse{Sessions: []*apiv1.MirrorSession{
		{SessionId: "remote-session", LastSequence: 7, Meta: &apiv1.MirrorSessionMeta{Command: "test"}},
	}}, nil
}

func (f *fakeTorqueAgent) GetSession(context.Context, *apiv1.MirrorGetSessionRequest) (*apiv1.MirrorSession, error) {
	return &apiv1.MirrorSession{SessionId: "remote-session", LastSequence: 7, Meta: &apiv1.MirrorSessionMeta{Command: "test"}}, nil
}

func (f *fakeTorqueAgent) Subscribe(req *apiv1.MirrorSubscribeRequest, stream apiv1.MirrorService_SubscribeServer) error {
	return stream.Send(&apiv1.MirrorFrame{
		SessionId: req.GetSessionId(),
		Producer:  "fake",
		Sequence:  req.GetFromSequence() + 1,
		Payload: &apiv1.MirrorFrame_Raw{
			Raw: []byte("frame"),
		},
	})
}

func (f *fakeTorqueAgent) SetSessionMeta(context.Context, *apiv1.MirrorSetSessionMetaRequest) (*apiv1.MirrorSession, error) {
	return &apiv1.MirrorSession{SessionId: "set"}, nil
}

func (f *fakeTorqueAgent) StreamLogs(req *apiv1.LogRequest, stream apiv1.LogService_StreamLogsServer) error {
	return stream.Send(&apiv1.LogLine{
		Namespace: req.GetNamespaces()[0],
		Pod:       "api-123",
		Container: "api",
		Raw:       "hello",
		Rendered:  "hello",
	})
}

func (f *fakeTorqueAgent) RunBuild(req *apiv1.RunBuildRequest, stream apiv1.BuildService_RunBuildServer) error {
	f.mu.Lock()
	f.builds = append(f.builds, req.GetOptions())
	f.mu.Unlock()
	if err := stream.Send(&apiv1.BuildEvent{Body: &apiv1.BuildEvent_Log{Log: &apiv1.LogLine{Raw: "build step", Rendered: "build step"}}}); err != nil {
		return err
	}
	return stream.Send(&apiv1.BuildEvent{Body: &apiv1.BuildEvent_Result{Result: &apiv1.BuildResult{Tags: req.GetOptions().GetTags(), Digest: "sha256:deadbeef"}}})
}

func (f *fakeTorqueAgent) lastBuild() *apiv1.BuildOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.builds) == 0 {
		return nil
	}
	return f.builds[len(f.builds)-1]
}

func (f *fakeTorqueAgent) Apply(req *apiv1.DeployApplyRequest, stream apiv1.DeployService_ApplyServer) error {
	return stream.Send(&apiv1.DeployEvent{Json: `{"kind":"summary","summary":{"release":"` + req.GetOptions().GetRelease() + `","status":"deployed"}}`})
}

func (f *fakeTorqueAgent) Destroy(req *apiv1.DeployDestroyRequest, stream apiv1.DeployService_DestroyServer) error {
	return stream.Send(&apiv1.DeployEvent{Json: `{"kind":"summary","summary":{"release":"` + req.GetOptions().GetRelease() + `","status":"deleted"}}`})
}

func (f *fakeTorqueAgent) Verify(req *apiv1.VerifyRequest, stream apiv1.VerifyService_VerifyServer) error {
	if err := stream.Send(&apiv1.VerifyEvent{Body: &apiv1.VerifyEvent_Progress{Progress: &apiv1.VerifyProgress{Phase: "evaluate"}}}); err != nil {
		return err
	}
	return stream.Send(&apiv1.VerifyEvent{Body: &apiv1.VerifyEvent_Done{Done: &apiv1.VerifyDone{Passed: true}}})
}

func (f *fakeStackAgent) Plan(context.Context, *apiv1.StackPlanRequest) (*apiv1.StackPlanResult, error) {
	return &apiv1.StackPlanResult{
		Json:      `{"stackName":"fake-stack","profile":"test","nodes":[{"id":"local/default/api","name":"api"}]}`,
		NodeCount: 1,
		StackName: "fake-stack",
		Profile:   "test",
	}, nil
}

func (f *fakeStackAgent) Apply(req *apiv1.StackRunRequest, stream apiv1.StackService_ApplyServer) error {
	return stream.Send(&apiv1.StackEvent{
		Json:  `{"runId":"stack-run","type":"RUN_COMPLETED","message":"stack apply completed"}`,
		RunId: "stack-run",
		Type:  "RUN_COMPLETED",
	})
}

func (f *fakeStackAgent) Delete(req *apiv1.StackRunRequest, stream apiv1.StackService_DeleteServer) error {
	return stream.Send(&apiv1.StackEvent{
		Json:  `{"runId":"stack-run","type":"RUN_COMPLETED","message":"stack delete completed"}`,
		RunId: "stack-run",
		Type:  "RUN_COMPLETED",
	})
}

func (f *fakeStackAgent) Status(context.Context, *apiv1.StackStatusRequest) (*apiv1.StackStatusResult, error) {
	return &apiv1.StackStatusResult{
		Json:  `{"runId":"stack-run","status":"succeeded","totals":{"planned":1,"succeeded":1}}`,
		RunId: "stack-run",
	}, nil
}

func startFakeAgent(t *testing.T) (addr string, stop func()) {
	addr, _, stop = startFakeAgentWithHandle(t)
	return addr, stop
}

func startFakeAgentWithHandle(t *testing.T) (addr string, fake *fakeTorqueAgent, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	fake = &fakeTorqueAgent{}
	apiv1.RegisterAgentInfoServiceServer(srv, fake)
	apiv1.RegisterMirrorServiceServer(srv, fake)
	apiv1.RegisterLogServiceServer(srv, fake)
	apiv1.RegisterBuildServiceServer(srv, fake)
	apiv1.RegisterDeployServiceServer(srv, fake)
	apiv1.RegisterVerifyServiceServer(srv, fake)
	apiv1.RegisterStackServiceServer(srv, &fakeStackAgent{})
	go func() {
		_ = srv.Serve(ln)
	}()
	return ln.Addr().String(), fake, func() {
		srv.Stop()
		_ = ln.Close()
	}
}

func TestMCPInitializeListAndInfo(t *testing.T) {
	addr, stop := startFakeAgent(t)
	defer stop()
	srv := New(Config{RemoteAgent: addr})

	init := rpc(t, srv, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if init.Error != nil {
		t.Fatalf("initialize error: %+v", init.Error)
	}
	if got := nestedString(init.Result, "protocolVersion"); got != ProtocolVersion {
		t.Fatalf("protocolVersion=%q", got)
	}

	list := rpc(t, srv, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if list.Error != nil {
		t.Fatalf("tools/list error: %+v", list.Error)
	}
	raw, _ := json.Marshal(list.Result)
	if !strings.Contains(string(raw), "torque.info") || !strings.Contains(string(raw), "torque.build.run") || !strings.Contains(string(raw), "torque.cache.plan") {
		t.Fatalf("tools/list missing expected tools: %s", raw)
	}

	info := callTool(t, srv, "torque.info", map[string]any{})
	if info.IsError {
		t.Fatalf("torque.info returned tool error: %+v", info.StructuredContent)
	}
	raw, _ = json.Marshal(info.StructuredContent)
	if !strings.Contains(string(raw), "remote-test") {
		t.Fatalf("torque.info did not include remote metadata: %s", raw)
	}
}

func TestCacheAdvisorTools(t *testing.T) {
	addr, fake, stop := startFakeAgentWithHandle(t)
	defer stop()
	srv := New(Config{RemoteAgent: addr, EnableWrite: true, MaxEventsReturned: 10})

	cacheDir := t.TempDir()
	intelDir := filepath.Join(cacheDir, "torque-cache-intel")
	if err := os.MkdirAll(intelDir, 0o755); err != nil {
		t.Fatalf("create cache intel dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(intelDir, "report.json"), []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("write cache intel report: %v", err)
	}

	baseArgs := map[string]any{
		"contextDir":    "./services/api",
		"dockerfile":    "Dockerfile",
		"tags":          []string{"ghcr.io/acme/api:dev"},
		"platforms":     []string{"linux/amd64"},
		"s3Cache":       "s3://acme-build-cache/torque/main",
		"s3CacheRegion": "us-east-1",
		"s3CacheName":   "api-main",
		"s3CacheMode":   "max",
		"cacheDir":      cacheDir,
	}

	inspect := callTool(t, srv, "torque.cache.inspect", baseArgs)
	assertToolOK(t, "cache-inspect", inspect, "s3-enabled")
	inspectRaw, _ := json.Marshal(inspect.StructuredContent)
	for _, want := range []string{"type=s3", "api-main", "torque-cache-intel"} {
		if !strings.Contains(string(inspectRaw), want) {
			t.Fatalf("cache inspect missing %q: %s", want, inspectRaw)
		}
	}
	leakyArgs := map[string]any{
		"contextDir": ".",
		"cacheTo":    []string{"type=registry,ref=example.com/cache:latest,token=super-secret"},
		"safety":     map[string]any{"confirm": true},
	}
	leakyInspect := callTool(t, srv, "torque.cache.inspect", leakyArgs)
	assertToolOK(t, "leaky-cache-inspect", leakyInspect, "redacted")
	leakyRaw, _ := json.Marshal(leakyInspect.StructuredContent)
	if strings.Contains(string(leakyRaw), "super-secret") {
		t.Fatalf("cache inspect leaked cache credential: %s", leakyRaw)
	}
	leakyWarm := callTool(t, srv, "torque.cache.warm", leakyArgs)
	if !leakyWarm.IsError {
		t.Fatalf("expected cache warm with secret-like cache credentials to fail")
	}
	leakyWarmRaw, _ := json.Marshal(leakyWarm.StructuredContent)
	if strings.Contains(string(leakyWarmRaw), "super-secret") {
		t.Fatalf("cache warm error leaked cache credential: %s", leakyWarmRaw)
	}

	planArgs := map[string]any{}
	for key, value := range baseArgs {
		planArgs[key] = value
	}
	planArgs["changedPaths"] = []string{"go.mod", "cmd/api/main.go"}
	plan := callTool(t, srv, "torque.cache.plan", planArgs)
	assertToolOK(t, "cache-plan", plan, "torque.cache.warm")
	planRaw, _ := json.Marshal(plan.StructuredContent)
	for _, want := range []string{"dependency-layer", "source-layer", "BuildService.RunBuild"} {
		if !strings.Contains(string(planRaw), want) {
			t.Fatalf("cache plan missing %q: %s", want, planRaw)
		}
	}

	denied := callTool(t, New(Config{RemoteAgent: addr}), "torque.cache.warm", baseArgs)
	if !denied.IsError {
		t.Fatalf("expected cache warm without write enable/confirm to fail")
	}
	deniedRaw, _ := json.Marshal(denied.StructuredContent)
	if !strings.Contains(string(deniedRaw), "CONFIRMATION_REQUIRED") {
		t.Fatalf("unexpected cache warm denial: %s", deniedRaw)
	}

	warmArgs := map[string]any{}
	for key, value := range baseArgs {
		warmArgs[key] = value
	}
	warmArgs["safety"] = map[string]any{"confirm": true}
	warm := callTool(t, srv, "torque.cache.warm", warmArgs)
	assertToolOK(t, "cache-warm", warm, "sha256:deadbeef")

	lastBuild := fake.lastBuild()
	if lastBuild == nil {
		t.Fatalf("cache warm did not call remote BuildService")
	}
	if lastBuild.GetPush() || lastBuild.GetLoad() {
		t.Fatalf("cache warm should not push or load: %+v", lastBuild)
	}
	joinedCache := strings.Join(append(lastBuild.GetCacheFrom(), lastBuild.GetCacheTo()...), "\n")
	for _, want := range []string{"type=s3", "bucket=acme-build-cache", "name=api-main", "mode=max"} {
		if !strings.Contains(joinedCache, want) {
			t.Fatalf("remote build cache settings missing %q: %s", want, joinedCache)
		}
	}
}

func TestRemoteGRPCToolsThroughMCP(t *testing.T) {
	addr, stop := startFakeAgent(t)
	defer stop()
	srv := New(Config{RemoteAgent: addr, EnableWrite: true, MaxEventsReturned: 10, MaxLogLinesReturned: 10})

	logs := callTool(t, srv, "torque.logs.query", map[string]any{
		"podQuery":   "api-.*",
		"namespaces": []string{"default"},
		"tailLines":  1,
	})
	assertToolOK(t, "logs", logs, "hello")

	build := callTool(t, srv, "torque.build.run", map[string]any{
		"contextDir": ".",
		"tags":       []string{"example/api:test"},
	})
	assertToolOK(t, "build", build, "sha256:deadbeef")

	verify := callTool(t, srv, "torque.verify.chart", map[string]any{
		"chart":     "./chart",
		"release":   "api",
		"namespace": "default",
	})
	assertToolOK(t, "verify", verify, "passed")

	apply := callTool(t, srv, "torque.apply.run", map[string]any{
		"chart":     "./chart",
		"release":   "api",
		"namespace": "default",
		"dryRun":    true,
	})
	assertToolOK(t, "apply", apply, "deployed")

	del := callTool(t, srv, "torque.delete.run", map[string]any{
		"release":   "api",
		"namespace": "default",
		"dryRun":    true,
	})
	assertToolOK(t, "delete", del, "deleted")

	stackPlan := callTool(t, srv, "torque.stack.plan", map[string]any{
		"root":    ".",
		"profile": "test",
	})
	assertToolOK(t, "stack-plan", stackPlan, "fake-stack")

	stackApply := callTool(t, srv, "torque.stack.apply", map[string]any{
		"root":   ".",
		"dryRun": true,
	})
	assertToolOK(t, "stack-apply", stackApply, "stack apply completed")

	stackDelete := callTool(t, srv, "torque.stack.delete", map[string]any{
		"root": ".",
		"safety": map[string]any{
			"confirm": true,
		},
	})
	assertToolOK(t, "stack-delete", stackDelete, "stack delete completed")

	stackStatus := callTool(t, srv, "torque.stack.status", map[string]any{
		"root":  ".",
		"runId": "stack-run",
	})
	assertToolOK(t, "stack-status", stackStatus, "succeeded")

	sessions := callTool(t, srv, "torque.session.list", map[string]any{"limit": 10})
	assertToolOK(t, "sessions", sessions, "remote-session")

	tail := callTool(t, srv, "torque.session.tail", map[string]any{"sessionId": "remote-session", "limit": 1})
	assertToolOK(t, "tail", tail, "frame")
}

func TestMCPBridgeBuildAndShipToRemoteTorque(t *testing.T) {
	addr, stop := startFakeAgent(t)
	defer stop()

	evidenceDir := t.TempDir()
	fakeTorque, argsFile := writeFakeTorqueBinary(t)
	t.Setenv("TORQUE_MCP_TORQUE_BIN", fakeTorque)

	srv := New(Config{RemoteAgent: addr, RemoteToken: "bridge-token", EnableWrite: true, MaxEventsReturned: 10, MaxLogLinesReturned: 10})

	build := callTool(t, srv, "torque.build.run", map[string]any{
		"contextDir": ".",
		"tags":       []string{"registry.local/torque-mcp:e2e"},
		"push":       false,
	})
	assertToolOK(t, "bridge-build", build, "sha256:deadbeef")

	ship := callTool(t, srv, "torque.ship.run", map[string]any{
		"chart":        "./chart",
		"release":      "torque-mcp",
		"namespace":    "mcp-e2e",
		"buildContext": ".",
		"tags":         []string{"registry.local/torque-mcp:e2e"},
		"push":         false,
		"skipVerify":   true,
		"skipExplain":  true,
		"evidenceDir":  evidenceDir,
		"safety": map[string]any{
			"confirm": true,
		},
	})
	assertToolOK(t, "bridge-ship", ship, "ship summary from fake torque")

	rawArgs, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read fake torque args: %v", err)
	}
	got := string(rawArgs)
	for _, want := range []string{"--remote-agent " + addr, "ship", "--chart ./chart", "--release torque-mcp", "--build .", "--tag registry.local/torque-mcp:e2e", "--yes"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fake torque args missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "bridge-token") || strings.Contains(got, "--remote-token") {
		t.Fatalf("remote token leaked through argv: %s", got)
	}
	rawResponse, _ := json.Marshal(ship.StructuredContent)
	if strings.Contains(string(rawResponse), "bridge-token") {
		t.Fatalf("remote token leaked through MCP response: %s", rawResponse)
	}
	shipContent, _ := ship.StructuredContent.(map[string]any)
	if stdout, _ := shipContent["stdout"].(string); !strings.Contains(stdout, "<redacted>") {
		t.Fatalf("expected redacted child output in MCP response: %s", rawResponse)
	}
	sessionID, _ := shipContent["sessionId"].(string)
	tail := callTool(t, srv, "torque.session.tail", map[string]any{"sessionId": sessionID, "limit": 10})
	tailRaw, _ := json.Marshal(tail.StructuredContent)
	if strings.Contains(string(tailRaw), "bridge-token") {
		t.Fatalf("remote token leaked through MCP session tail: %s", tailRaw)
	}
}

func TestMutatingToolRequiresConfirmation(t *testing.T) {
	addr, stop := startFakeAgent(t)
	defer stop()
	srv := New(Config{RemoteAgent: addr})
	res := callTool(t, srv, "torque.apply.run", map[string]any{
		"chart":     "./chart",
		"release":   "api",
		"namespace": "default",
	})
	if !res.IsError {
		t.Fatalf("expected apply without dryRun/confirm to fail")
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(raw), "CONFIRMATION_REQUIRED") {
		t.Fatalf("unexpected error: %s", raw)
	}

	ship := callTool(t, srv, "torque.ship.run", map[string]any{
		"chart":       "./chart",
		"release":     "api",
		"skipBuild":   true,
		"skipVerify":  true,
		"skipExplain": true,
		"evidenceDir": t.TempDir(),
	})
	if !ship.IsError {
		t.Fatalf("expected ship without write enable/confirm to fail")
	}
	raw, _ = json.Marshal(ship.StructuredContent)
	if !strings.Contains(string(raw), "CONFIRMATION_REQUIRED") {
		t.Fatalf("unexpected ship error: %s", raw)
	}
}

func TestMCPHTTPRequiresBearerTokenWhenConfigured(t *testing.T) {
	srv := New(Config{AuthToken: "mcp-secret"})
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	rec := httptest.NewRecorder()
	srv.handleHTTPMCP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized without token, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	req.Header.Set("Authorization", "Bearer mcp-secret")
	rec = httptest.NewRecorder()
	srv.handleHTTPMCP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected success with token, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "torque.info") {
		t.Fatalf("expected tools/list response, got: %s", rec.Body.String())
	}
}

func writeFakeTorqueBinary(t *testing.T) (path string, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args.txt")
	path = filepath.Join(dir, "torque")
	script := `#!/bin/sh
set -eu
printf '%s ' "$@" > "$TORQUE_FAKE_ARGS"
echo "child received token=$TORQUE_REMOTE_TOKEN"
evidence=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "--evidence-dir" ]; then
    evidence="$arg"
  fi
  case "$arg" in
    --evidence-dir=*) evidence="${arg#--evidence-dir=}" ;;
  esac
  prev="$arg"
done
if [ -n "$evidence" ]; then
  mkdir -p "$evidence"
  cat > "$evidence/ship.json" <<'JSON'
{"tool":"torque ship","status":"succeeded","message":"ship summary from fake torque"}
JSON
fi
echo "ship summary from fake torque"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake torque: %v", err)
	}
	t.Setenv("TORQUE_FAKE_ARGS", argsFile)
	return path, argsFile
}

type decodedRPC struct {
	Result map[string]any `json:"result"`
	Error  *rpcError      `json:"error"`
}

func rpc(t *testing.T, srv *Server, msg string) decodedRPC {
	t.Helper()
	resp, ok := srv.handleMessage(context.Background(), []byte(msg))
	if !ok {
		t.Fatalf("no response for %s", msg)
	}
	var out decodedRPC
	if err := json.Unmarshal(resp, &out); err != nil {
		t.Fatalf("decode rpc response: %v\n%s", err, resp)
	}
	return out
}

func callTool(t *testing.T, srv *Server, name string, args map[string]any) toolResult {
	t.Helper()
	params := map[string]any{"name": name, "arguments": args}
	req := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": params}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	resp, ok := srv.handleMessage(context.Background(), raw)
	if !ok {
		t.Fatalf("no response for tools/call %s", name)
	}
	var decoded struct {
		Result toolResult `json:"result"`
		Error  *rpcError  `json:"error"`
	}
	if err := json.Unmarshal(resp, &decoded); err != nil {
		t.Fatalf("decode tools/call response: %v\n%s", err, resp)
	}
	if decoded.Error != nil {
		t.Fatalf("tools/call protocol error: %+v", decoded.Error)
	}
	return decoded.Result
}

func assertToolOK(t *testing.T, label string, res toolResult, contains string) {
	t.Helper()
	if res.IsError {
		t.Fatalf("%s returned tool error: %+v", label, res.StructuredContent)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(strings.ToLower(string(raw)), strings.ToLower(contains)) {
		t.Fatalf("%s output did not contain %q: %s", label, contains, raw)
	}
}

func nestedString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
