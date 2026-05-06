package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/ingresslabs/torque/internal/grpcutil"
	"github.com/ingresslabs/torque/internal/workflows/buildsvc"
	apiv1 "github.com/ingresslabs/torque/pkg/api/torque/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func (s *Server) dialRemote(ctx context.Context) (*grpc.ClientConn, error) {
	if err := requireRemote(strings.TrimSpace(s.cfg.RemoteAgent)); err != nil {
		return nil, err
	}
	creds, err := s.cfg.remoteCredentials()
	if err != nil {
		return nil, err
	}
	return grpcutil.Dial(ctx, strings.TrimSpace(s.cfg.RemoteAgent),
		grpc.WithTransportCredentials(creds),
		grpcutil.WithBearerToken(s.cfg.RemoteToken),
	)
}

func protoMap(msg proto.Message) map[string]any {
	if msg == nil {
		return nil
	}
	raw, err := protojson.MarshalOptions{UseProtoNames: false, EmitUnpopulated: false}.Marshal(msg)
	if err != nil {
		return map[string]any{"marshalError": err.Error()}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"raw": string(raw)}
	}
	return out
}

type commonArgs struct {
	Kube struct {
		Context    string `json:"context"`
		Kubeconfig string `json:"kubeconfig"`
		Namespace  string `json:"namespace"`
	} `json:"kube"`
	Session struct {
		ID        string `json:"id"`
		Requester string `json:"requester"`
	} `json:"session"`
	Safety struct {
		Confirm        bool `json:"confirm"`
		NonInteractive bool `json:"nonInteractive"`
		DryRun         bool `json:"dryRun"`
	} `json:"safety"`
	Wait struct {
		Mode           string `json:"mode"`
		TimeoutSeconds int    `json:"timeoutSeconds"`
	} `json:"wait"`
}

func (a commonArgs) timeout(defaultSeconds int) int {
	if a.Wait.TimeoutSeconds > 0 {
		return a.Wait.TimeoutSeconds
	}
	return defaultSeconds
}

func (a commonArgs) requester() string {
	if strings.TrimSpace(a.Session.Requester) != "" {
		return strings.TrimSpace(a.Session.Requester)
	}
	return "mcp"
}

func (a commonArgs) sessionID(fallback string) string {
	if strings.TrimSpace(a.Session.ID) != "" {
		return strings.TrimSpace(a.Session.ID)
	}
	return fallback
}

func (s *Server) mirrorSetMeta(ctx context.Context, conn *grpc.ClientConn, sessionID string, meta *apiv1.MirrorSessionMeta, tags map[string]string) {
	if conn == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	_, _ = apiv1.NewMirrorServiceClient(conn).SetSessionMeta(ctx, &apiv1.MirrorSetSessionMetaRequest{
		SessionId: sessionID,
		Meta:      meta,
		Tags:      tags,
	})
}

func (s *Server) recordRemoteSession(id, kind, requester string) *sessionRecord {
	rec := &sessionRecord{
		SessionID: strings.TrimSpace(id),
		Kind:      kind,
		State:     sessionRunning,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Requester: requester,
		Remote:    true,
	}
	s.sessions.upsert(rec)
	return rec
}

func (s *Server) finishSession(id string, err error, summary map[string]any) {
	if err != nil {
		s.sessions.update(id, sessionFailed, err.Error(), summary)
		return
	}
	s.sessions.update(id, sessionSucceeded, "", summary)
}

func recvEOF(err error) bool {
	return err == io.EOF
}

func buildOptionsFromArgs(a buildRunArgs) buildsvc.Options {
	return buildsvc.Options{
		ContextDir: firstNonEmpty(a.ContextDir, "."),
		Dockerfile: firstNonEmpty(a.Dockerfile, "Dockerfile"),
		Tags:       append([]string(nil), a.Tags...),
		Platforms:  append([]string(nil), a.Platforms...),
		BuildArgs:  append([]string(nil), a.BuildArgs...),
		Secrets:    append([]string(nil), a.Secrets...),
		CacheFrom:  append([]string(nil), a.CacheFrom...),
		CacheTo:    append([]string(nil), a.CacheTo...),
		S3Cache: buildsvc.S3CacheOptions{
			Ref:          a.S3Cache,
			Region:       a.S3CacheRegion,
			Name:         a.S3CacheName,
			Mode:         a.S3CacheMode,
			EndpointURL:  a.S3CacheEndpoint,
			UsePathStyle: a.S3CachePathStyle,
		},
		Push:               a.Push,
		Load:               a.Load,
		NoCache:            a.NoCache,
		Builder:            a.Builder,
		DockerContext:      a.DockerContext,
		CacheDir:           a.CacheDir,
		BuildMode:          firstNonEmpty(a.Mode, string(buildsvc.ModeAuto)),
		ComposeFiles:       append([]string(nil), a.ComposeFiles...),
		ComposeProfiles:    append([]string(nil), a.ComposeProfiles...),
		ComposeServices:    append([]string(nil), a.ComposeServices...),
		ComposeProject:     a.ComposeProject,
		AuthFile:           a.AuthFile,
		SandboxConfig:      a.SandboxConfig,
		SandboxBin:         a.SandboxBin,
		SandboxBinds:       append([]string(nil), a.SandboxBinds...),
		SandboxWorkdir:     a.SandboxWorkdir,
		SandboxLogs:        a.SandboxLogs,
		LogFile:            a.LogFile,
		RemoveIntermediate: true,
		Quiet:              true,
		Output:             "logs",
	}
}

func mapErrorResult(code string, err error, retryable bool, hints ...string) toolResult {
	if err == nil {
		err = errors.New(code)
	}
	return textToolError(code, err.Error(), retryable, hints, nil)
}
