package main

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	apiv1 "github.com/ingresslabs/torque/pkg/api/torque/api/v1"
	"google.golang.org/grpc"
)

type fakeCLIStackService struct {
	apiv1.UnimplementedStackServiceServer
}

func (f *fakeCLIStackService) Plan(context.Context, *apiv1.StackPlanRequest) (*apiv1.StackPlanResult, error) {
	return &apiv1.StackPlanResult{
		Json:      `{"stackName":"remote-cli","profile":"test","nodes":[]}`,
		NodeCount: 0,
		StackName: "remote-cli",
		Profile:   "test",
	}, nil
}

func (f *fakeCLIStackService) Apply(req *apiv1.StackRunRequest, stream apiv1.StackService_ApplyServer) error {
	return stream.Send(&apiv1.StackEvent{
		Json:  `{"runId":"remote-run","type":"RUN_COMPLETED","message":"remote stack apply completed"}`,
		RunId: "remote-run",
		Type:  "RUN_COMPLETED",
	})
}

func (f *fakeCLIStackService) Delete(req *apiv1.StackRunRequest, stream apiv1.StackService_DeleteServer) error {
	return stream.Send(&apiv1.StackEvent{
		Json:  `{"runId":"remote-run","type":"RUN_COMPLETED","message":"remote stack delete completed"}`,
		RunId: "remote-run",
		Type:  "RUN_COMPLETED",
	})
}

func (f *fakeCLIStackService) Status(context.Context, *apiv1.StackStatusRequest) (*apiv1.StackStatusResult, error) {
	return &apiv1.StackStatusResult{
		Json:  `{"runId":"remote-run","status":"succeeded","totals":{"planned":0,"succeeded":0}}`,
		RunId: "remote-run",
	}, nil
}

func TestStackRemoteAgentCommandsUseStackService(t *testing.T) {
	addr, stop := startFakeCLIStackService(t)
	defer stop()

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "plan", args: []string{"--remote-agent", addr, "stack", "plan", "--output", "json"}, want: "remote-cli"},
		{name: "apply", args: []string{"--remote-agent", addr, "stack", "--output", "json", "apply", "--dry-run"}, want: "remote stack apply completed"},
		{name: "status", args: []string{"--remote-agent", addr, "stack", "status", "--format", "json"}, want: "succeeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newRootCommand()
			var out bytes.Buffer
			var errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs(tc.args)
			if err := cmd.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("execute: %v (stdout=%q stderr=%q)", err, out.String(), errOut.String())
			}
			got := out.String() + errOut.String()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected output to contain %q, got stdout=%q stderr=%q", tc.want, out.String(), errOut.String())
			}
		})
	}
}

func startFakeCLIStackService(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	apiv1.RegisterStackServiceServer(srv, &fakeCLIStackService{})
	go func() {
		_ = srv.Serve(ln)
	}()
	return ln.Addr().String(), func() {
		srv.Stop()
		_ = ln.Close()
	}
}
