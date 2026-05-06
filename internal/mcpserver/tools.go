package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ingresslabs/torque/internal/api/convert"
	"github.com/ingresslabs/torque/internal/capture"
	"github.com/ingresslabs/torque/internal/workflows/buildsvc"
	apiv1 "github.com/ingresslabs/torque/pkg/api/torque/api/v1"
)

type buildRunArgs struct {
	commonArgs
	ContextDir       string   `json:"contextDir"`
	Dockerfile       string   `json:"dockerfile"`
	Tags             []string `json:"tags"`
	Platforms        []string `json:"platforms"`
	BuildArgs        []string `json:"buildArgs"`
	Secrets          []string `json:"secrets"`
	CacheFrom        []string `json:"cacheFrom"`
	CacheTo          []string `json:"cacheTo"`
	S3Cache          string   `json:"s3Cache"`
	S3CacheRegion    string   `json:"s3CacheRegion"`
	S3CacheName      string   `json:"s3CacheName"`
	S3CacheMode      string   `json:"s3CacheMode"`
	S3CacheEndpoint  string   `json:"s3CacheEndpointUrl"`
	S3CachePathStyle bool     `json:"s3CachePathStyle"`
	Push             bool     `json:"push"`
	Load             bool     `json:"load"`
	NoCache          bool     `json:"noCache"`
	Builder          string   `json:"builder"`
	DockerContext    string   `json:"dockerContext"`
	CacheDir         string   `json:"cacheDir"`
	Mode             string   `json:"mode"`
	ComposeFiles     []string `json:"composeFiles"`
	ComposeProfiles  []string `json:"composeProfiles"`
	ComposeServices  []string `json:"composeServices"`
	ComposeProject   string   `json:"composeProject"`
	AuthFile         string   `json:"authFile"`
	SandboxConfig    string   `json:"sandboxConfig"`
	SandboxBin       string   `json:"sandboxBin"`
	SandboxBinds     []string `json:"sandboxBinds"`
	SandboxWorkdir   string   `json:"sandboxWorkdir"`
	SandboxLogs      bool     `json:"sandboxLogs"`
	LogFile          string   `json:"logFile"`
	Sign             bool     `json:"sign"`
}

type logsQueryArgs struct {
	commonArgs
	PodQuery          string   `json:"podQuery"`
	Namespaces        []string `json:"namespaces"`
	AllNamespaces     bool     `json:"allNamespaces"`
	LabelSelector     string   `json:"labelSelector"`
	FieldSelector     string   `json:"fieldSelector"`
	Containers        []string `json:"containers"`
	ExcludeContainers []string `json:"excludeContainers"`
	ExcludePods       []string `json:"excludePods"`
	HighlightTerms    []string `json:"highlightTerms"`
	IncludeEvents     bool     `json:"includeEvents"`
	EventsOnly        bool     `json:"eventsOnly"`
	TailLines         int64    `json:"tailLines"`
	Follow            bool     `json:"follow"`
	Timestamps        bool     `json:"timestamps"`
	Template          string   `json:"template"`
}

type applyRunArgs struct {
	commonArgs
	Chart           string   `json:"chart"`
	Release         string   `json:"release"`
	Namespace       string   `json:"namespace"`
	Version         string   `json:"version"`
	ValuesFiles     []string `json:"valuesFiles"`
	Set             []string `json:"set"`
	SetString       []string `json:"setString"`
	SetFile         []string `json:"setFile"`
	HelmWait        *bool    `json:"helmWait"`
	Atomic          *bool    `json:"atomic"`
	UpgradeOnly     bool     `json:"upgradeOnly"`
	CreateNamespace bool     `json:"createNamespace"`
	DryRun          bool     `json:"dryRun"`
	Diff            bool     `json:"diff"`
	TimeoutSeconds  int64    `json:"timeoutSeconds"`
}

type deleteRunArgs struct {
	commonArgs
	Release        string `json:"release"`
	Namespace      string `json:"namespace"`
	HelmWait       *bool  `json:"helmWait"`
	KeepHistory    bool   `json:"keepHistory"`
	DryRun         bool   `json:"dryRun"`
	Force          bool   `json:"force"`
	DisableHooks   bool   `json:"disableHooks"`
	TimeoutSeconds int64  `json:"timeoutSeconds"`
}

type verifyArgs struct {
	commonArgs
	Target      string   `json:"target"`
	Chart       string   `json:"chart"`
	Release     string   `json:"release"`
	Namespace   string   `json:"namespace"`
	Version     string   `json:"version"`
	ValuesFiles []string `json:"valuesFiles"`
	Set         []string `json:"set"`
	SetString   []string `json:"setString"`
	SetFile     []string `json:"setFile"`
	Mode        string   `json:"mode"`
	FailOn      string   `json:"failOn"`
	Format      string   `json:"format"`
	RulesDir    string   `json:"rulesDir"`
	Policy      string   `json:"policy"`
	PolicyMode  string   `json:"policyMode"`
	Baseline    string   `json:"baseline"`
}

type stackArgs struct {
	commonArgs
	Config                     string           `json:"config"`
	Root                       string           `json:"root"`
	Profile                    string           `json:"profile"`
	Clusters                   []string         `json:"clusters"`
	Tags                       []string         `json:"tags"`
	FromPaths                  []string         `json:"fromPaths"`
	Releases                   []string         `json:"releases"`
	GitRange                   string           `json:"gitRange"`
	GitIncludeDeps             *bool            `json:"gitIncludeDeps"`
	GitIncludeDependents       *bool            `json:"gitIncludeDependents"`
	IncludeDeps                *bool            `json:"includeDeps"`
	IncludeDependents          *bool            `json:"includeDependents"`
	AllowMissingDeps           *bool            `json:"allowMissingDeps"`
	InferDeps                  *bool            `json:"inferDeps"`
	InferConfigRefs            *bool            `json:"inferConfigRefs"`
	SecretProvider             string           `json:"secretProvider"`
	SecretConfig               string           `json:"secretConfig"`
	Bundle                     string           `json:"bundle"`
	Concurrency                int              `json:"concurrency"`
	ProgressiveConcurrency     *bool            `json:"progressiveConcurrency"`
	FailFast                   *bool            `json:"failFast"`
	ContinueOnError            *bool            `json:"continueOnError"`
	Yes                        bool             `json:"yes"`
	DryRun                     bool             `json:"dryRun"`
	Diff                       bool             `json:"diff"`
	CacheApply                 bool             `json:"cacheApply"`
	HelmLogs                   bool             `json:"helmLogs"`
	RunID                      string           `json:"runId"`
	Retry                      int              `json:"retry"`
	KubeQPS                    float32          `json:"kubeQps"`
	KubeBurst                  int              `json:"kubeBurst"`
	MaxParallelPerNamespace    int              `json:"maxParallelPerNamespace"`
	MaxParallelKind            map[string]int32 `json:"maxParallelKind"`
	ParallelismGroupLimit      int              `json:"parallelismGroupLimit"`
	Lock                       *bool            `json:"lock"`
	TakeoverLock               *bool            `json:"takeoverLock"`
	LockTTLSeconds             int64            `json:"lockTtlSeconds"`
	LockOwner                  string           `json:"lockOwner"`
	FailMode                   string           `json:"failMode"`
	AdaptiveMin                int              `json:"adaptiveMin"`
	AdaptiveWindow             int              `json:"adaptiveWindow"`
	AdaptiveRampSuccesses      int              `json:"adaptiveRampSuccesses"`
	AdaptiveRampMaxFailureRate float64          `json:"adaptiveRampMaxFailureRate"`
	AdaptiveCooldownSevere     int              `json:"adaptiveCooldownSevere"`
	Limit                      int              `json:"limit"`
	Format                     string           `json:"format"`
}

type shipRunArgs struct {
	commonArgs
	Chart            string   `json:"chart"`
	Release          string   `json:"release"`
	Namespace        string   `json:"namespace"`
	Version          string   `json:"version"`
	ValuesFiles      []string `json:"valuesFiles"`
	Set              []string `json:"set"`
	SetString        []string `json:"setString"`
	SetFile          []string `json:"setFile"`
	SecretProvider   string   `json:"secretProvider"`
	SecretConfig     string   `json:"secretConfig"`
	BuildContext     string   `json:"buildContext"`
	Dockerfile       string   `json:"dockerfile"`
	Tags             []string `json:"tags"`
	Platforms        []string `json:"platforms"`
	BuildArgs        []string `json:"buildArgs"`
	BuildSecrets     []string `json:"buildSecrets"`
	CacheFrom        []string `json:"cacheFrom"`
	CacheTo          []string `json:"cacheTo"`
	S3Cache          string   `json:"s3Cache"`
	S3CacheRegion    string   `json:"s3CacheRegion"`
	S3CacheName      string   `json:"s3CacheName"`
	S3CacheMode      string   `json:"s3CacheMode"`
	S3CacheEndpoint  string   `json:"s3CacheEndpointUrl"`
	S3CachePathStyle bool     `json:"s3CachePathStyle"`
	Push             *bool    `json:"push"`
	Load             bool     `json:"load"`
	NoCache          bool     `json:"noCache"`
	Attest           *bool    `json:"attest"`
	SBOM             bool     `json:"sbom"`
	Provenance       bool     `json:"provenance"`
	AttestDir        string   `json:"attestDir"`
	Hermetic         bool     `json:"hermetic"`
	AllowNetwork     bool     `json:"allowNetwork"`
	AllowUnpinned    bool     `json:"allowUnpinnedBases"`
	BuildPolicy      string   `json:"buildPolicy"`
	BuildPolicyMode  string   `json:"buildPolicyMode"`
	Builder          string   `json:"builder"`
	AuthFile         string   `json:"authFile"`
	Sandbox          bool     `json:"sandbox"`
	SandboxConfig    string   `json:"sandboxConfig"`
	BuildOutput      string   `json:"buildOutput"`
	VerifyMode       string   `json:"verifyMode"`
	VerifyFailOn     string   `json:"verifyFailOn"`
	EvidenceDir      string   `json:"evidenceDir"`
	CaptureTags      []string `json:"captureTags"`
	NoCapture        bool     `json:"noCapture"`
	CreateNamespace  bool     `json:"createNamespace"`
	HelmWait         *bool    `json:"helmWait"`
	Atomic           *bool    `json:"atomic"`
	Yes              bool     `json:"yes"`
	PlanOnly         bool     `json:"planOnly"`
	SkipBuild        bool     `json:"skipBuild"`
	SkipVerify       bool     `json:"skipVerify"`
	SkipExplain      bool     `json:"skipExplain"`
	TimeoutSeconds   int      `json:"timeoutSeconds"`
}

func (s *Server) registerTools() {
	s.register("torque.info", "Torque Info", "Return torque-mcp, local Torque, and optional remote torque-agent metadata.", readOnlyAnnotations("Torque Info", false), objectSchema(map[string]any{}), s.toolInfo)
	s.register("torque.session.list", "List Sessions", "List local MCP sessions and remote MirrorService sessions when a remote agent is configured.", readOnlyAnnotations("List Sessions", true), objectSchema(map[string]any{"limit": intSchema("Maximum sessions to return")}), s.toolSessionList)
	s.register("torque.session.get", "Get Session", "Get a local MCP session or remote MirrorService session by ID.", readOnlyAnnotations("Get Session", true), objectSchema(map[string]any{"sessionId": stringSchema("Session ID")}, "sessionId"), s.toolSessionGet)
	s.register("torque.session.tail", "Tail Session", "Read a bounded page of local or remote session events.", readOnlyAnnotations("Tail Session", true), objectSchema(map[string]any{
		"sessionId":    stringSchema("Session ID"),
		"fromSequence": intSchema("First sequence to read after"),
		"limit":        intSchema("Maximum events to return"),
	}, "sessionId"), s.toolSessionTail)
	s.register("torque.logs.query", "Query Logs", "Stream logs through remote torque-agent LogService.", readOnlyAnnotations("Query Logs", true), anyObjectSchema(), s.toolLogsQuery)
	s.register("torque.build.run", "Run Build", "Run a BuildKit/Compose build through remote torque-agent BuildService.", writeAnnotations("Run Build", false), anyObjectSchema(), s.toolBuildRun)
	s.register("torque.cache.inspect", "Inspect Build Cache", "Return normalized BuildKit cache imports/exports, S3 cache manifest settings, and local Torque cache-intel evidence without scraping logs.", readOnlyAnnotations("Inspect Build Cache", false), anyObjectSchema(), s.toolCacheInspect)
	s.register("torque.cache.plan", "Plan Build Cache", "Plan cache reuse and warm targets from changed paths, build inputs, and first-class S3 cache settings.", readOnlyAnnotations("Plan Build Cache", false), anyObjectSchema(), s.toolCachePlan)
	s.register("torque.cache.warm", "Warm Build Cache", "Run a confirmed remote build that writes configured BuildKit cache exports, including first-class S3 cache exports.", writeAnnotations("Warm Build Cache", false), anyObjectSchema(), s.toolCacheWarm)
	s.register("torque.ship.run", "Ship Release", "Run the Torque ship workflow and forward configured remote-agent settings to the target Torque agent.", writeAnnotations("Ship Release", false), anyObjectSchema(), s.toolShipRun)
	s.register("torque.apply.run", "Apply Release", "Run Helm apply through remote torque-agent DeployService.", writeAnnotations("Apply Release", false), anyObjectSchema(), s.toolApplyRun)
	s.register("torque.delete.run", "Delete Release", "Run Helm delete through remote torque-agent DeployService.", writeAnnotations("Delete Release", true), anyObjectSchema(), s.toolDeleteRun)
	s.register("torque.verify.chart", "Verify Chart", "Verify a chart through remote torque-agent VerifyService.", readOnlyAnnotations("Verify Chart", true), anyObjectSchema(), s.toolVerifyChart)
	s.register("torque.verify.namespace", "Verify Namespace", "Verify a live namespace through remote torque-agent VerifyService.", readOnlyAnnotations("Verify Namespace", true), anyObjectSchema(), s.toolVerifyNamespace)
	s.register("torque.capture.summarize", "Summarize Capture", "Summarize a Torque SQLite evidence capture.", readOnlyAnnotations("Summarize Capture", false), objectSchema(map[string]any{
		"path":      stringSchema("Capture SQLite path"),
		"sessionId": stringSchema("Optional capture session ID"),
	}, "path"), s.toolCaptureSummarize)
	s.register("torque.capture.read_artifact", "Read Capture Artifact", "Read named artifacts from a Torque SQLite evidence capture.", readOnlyAnnotations("Read Capture Artifact", false), objectSchema(map[string]any{
		"path":  stringSchema("Capture SQLite path"),
		"names": stringArraySchema("Artifact names"),
	}, "path"), s.toolCaptureReadArtifact)
	s.register("torque.apply.plan", "Apply Plan", "Run local `torque apply plan --format json` and return the parsed plan when possible.", readOnlyAnnotations("Apply Plan", true), anyObjectSchema(), s.toolApplyPlan)
	s.register("torque.stack.plan", "Stack Plan", "Compile a stack plan locally or through remote torque-agent StackService.", readOnlyAnnotations("Stack Plan", false), anyObjectSchema(), s.toolStackPlan)
	s.register("torque.stack.apply", "Stack Apply", "Run stack apply through remote torque-agent StackService.", writeAnnotations("Stack Apply", false), anyObjectSchema(), s.toolStackApply)
	s.register("torque.stack.delete", "Stack Delete", "Run stack delete through remote torque-agent StackService.", writeAnnotations("Stack Delete", true), anyObjectSchema(), s.toolStackDelete)
	s.register("torque.stack.status", "Stack Status", "Read stack run status locally or through remote torque-agent StackService.", readOnlyAnnotations("Stack Status", true), anyObjectSchema(), s.toolStackStatus)
	s.register("torque.env.inspect", "Inspect Environment", "Return Torque-related environment and MCP server policy with sensitive values redacted.", readOnlyAnnotations("Inspect Environment", false), objectSchema(map[string]any{}), s.toolEnvInspect)
}

func (s *Server) register(name, title, description string, annotations map[string]any, input map[string]any, handler toolHandler) {
	s.tools[name] = toolSpec{
		def: toolDefinition{
			Name:         name,
			Title:        title,
			Description:  description,
			InputSchema:  input,
			OutputSchema: anyObjectSchema(),
			Annotations:  annotations,
		},
		handler: handler,
	}
}

func (s *Server) toolInfo(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	ctx, cancel := withTimeout(ctx, 10)
	defer cancel()
	info := map[string]any{
		"server": map[string]any{
			"name":            "torque-mcp",
			"protocolVersion": ProtocolVersion,
			"version":         s.version,
		},
		"mode": map[string]any{
			"remoteAgent":  strings.TrimSpace(s.cfg.RemoteAgent),
			"writeEnabled": s.cfg.EnableWrite,
		},
		"tools": sortedToolNames(s.tools),
	}
	if strings.TrimSpace(s.cfg.RemoteAgent) != "" {
		conn, err := s.dialRemote(ctx)
		if err != nil {
			info["remote"] = map[string]any{"agent": s.cfg.RemoteAgent, "error": err.Error()}
		} else {
			defer conn.Close()
			remoteInfo, rerr := apiv1.NewAgentInfoServiceClient(conn).GetInfo(ctx, &apiv1.AgentInfoRequest{})
			if rerr != nil {
				info["remote"] = map[string]any{"agent": s.cfg.RemoteAgent, "error": rerr.Error()}
			} else {
				info["remote"] = protoMap(remoteInfo)
			}
		}
	}
	return textResult("Torque MCP server is ready.", info, resourceLink("torque://info", "info", "Torque MCP server metadata", "application/json")), nil
}

func (s *Server) toolSessionList(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args struct {
		Limit int `json:"limit"`
	}
	_ = decodeArgs(raw, &args)
	out := map[string]any{"local": s.sessions.list()}
	if strings.TrimSpace(s.cfg.RemoteAgent) != "" {
		ctx, cancel := withTimeout(ctx, 10)
		defer cancel()
		conn, err := s.dialRemote(ctx)
		if err != nil {
			out["remoteError"] = err.Error()
		} else {
			defer conn.Close()
			resp, err := apiv1.NewMirrorServiceClient(conn).ListSessions(ctx, &apiv1.MirrorListSessionsRequest{Limit: int32(args.Limit)})
			if err != nil {
				out["remoteError"] = err.Error()
			} else {
				out["remote"] = protoMap(resp)
			}
		}
	}
	return textResult("Session list returned.", out, resourceLink("torque://sessions", "sessions", "Torque MCP sessions", "application/json")), nil
}

func (s *Server) toolSessionGet(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args struct {
		SessionID string `json:"sessionId"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	id := strings.TrimSpace(args.SessionID)
	if rec, ok := s.sessions.get(id); ok {
		return textResult("Session returned.", rec, resourceLink("torque://sessions/"+id, "session", "Local MCP session", "application/json")), nil
	}
	if strings.TrimSpace(s.cfg.RemoteAgent) == "" {
		return textToolError("SESSION_NOT_FOUND", "session not found locally and no remote agent is configured", false, nil, nil), nil
	}
	ctx, cancel := withTimeout(ctx, 10)
	defer cancel()
	conn, err := s.dialRemote(ctx)
	if err != nil {
		return mapErrorResult("REMOTE_DIAL_FAILED", err, true), nil
	}
	defer conn.Close()
	resp, err := apiv1.NewMirrorServiceClient(conn).GetSession(ctx, &apiv1.MirrorGetSessionRequest{SessionId: id})
	if err != nil {
		return mapErrorResult("REMOTE_SESSION_GET_FAILED", err, true), nil
	}
	return textResult("Remote session returned.", protoMap(resp), resourceLink("torque://sessions/"+id, "session", "Remote MirrorService session", "application/json")), nil
}

func (s *Server) toolSessionTail(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args struct {
		SessionID    string `json:"sessionId"`
		FromSequence uint64 `json:"fromSequence"`
		Limit        int    `json:"limit"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	if args.Limit <= 0 || args.Limit > s.cfg.MaxEventsReturned {
		args.Limit = s.cfg.MaxEventsReturned
	}
	if events := s.sessions.tail(args.SessionID, args.FromSequence, args.Limit); len(events) > 0 {
		return textResult("Local session events returned.", map[string]any{"events": events}, nilLink(args.SessionID)...), nil
	}
	if strings.TrimSpace(s.cfg.RemoteAgent) == "" {
		return textResult("No local events found.", map[string]any{"events": []any{}}, nilLink(args.SessionID)...), nil
	}
	ctx, cancel := withTimeout(ctx, 3)
	defer cancel()
	conn, err := s.dialRemote(ctx)
	if err != nil {
		return mapErrorResult("REMOTE_DIAL_FAILED", err, true), nil
	}
	defer conn.Close()
	stream, err := apiv1.NewMirrorServiceClient(conn).Subscribe(ctx, &apiv1.MirrorSubscribeRequest{
		SessionId:    args.SessionID,
		Replay:       true,
		FromSequence: args.FromSequence,
	})
	if err != nil {
		return mapErrorResult("REMOTE_SESSION_TAIL_FAILED", err, true), nil
	}
	frames := make([]map[string]any, 0, args.Limit)
	for len(frames) < args.Limit {
		frame, err := stream.Recv()
		if recvEOF(err) {
			break
		}
		if err != nil {
			break
		}
		frames = append(frames, protoMap(frame))
	}
	return textResult("Remote session frames returned.", map[string]any{"frames": frames}, nilLink(args.SessionID)...), nil
}

func (s *Server) toolLogsQuery(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args logsQueryArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	if strings.TrimSpace(args.PodQuery) == "" {
		return textToolError("INVALID_ARGUMENTS", "podQuery is required", false, nil, nil), nil
	}
	if args.TailLines <= 0 {
		args.TailLines = 100
	}
	if args.TailLines > int64(s.cfg.MaxLogLinesReturned) {
		args.TailLines = int64(s.cfg.MaxLogLinesReturned)
	}
	ctx, cancel := withTimeout(ctx, args.timeout(s.cfg.DefaultToolTimeoutSeconds))
	defer cancel()
	conn, err := s.dialRemote(ctx)
	if err != nil {
		return mapErrorResult("REMOTE_DIAL_FAILED", err, true), nil
	}
	defer conn.Close()
	rec := s.sessions.create("logs", args.requester(), true)
	sessionID := args.sessionID(rec.SessionID)
	s.recordRemoteSession(sessionID, "logs", args.requester())
	req := &apiv1.LogRequest{
		PodQuery:          args.PodQuery,
		Namespaces:        args.Namespaces,
		AllNamespaces:     args.AllNamespaces,
		LabelSelector:     args.LabelSelector,
		FieldSelector:     args.FieldSelector,
		Containers:        args.Containers,
		ExcludeContainers: args.ExcludeContainers,
		ExcludePods:       args.ExcludePods,
		HighlightTerms:    args.HighlightTerms,
		IncludeEvents:     args.IncludeEvents,
		EventsOnly:        args.EventsOnly,
		TailLines:         args.TailLines,
		Follow:            args.Follow,
		Timestamps:        args.Timestamps,
		Template:          args.Template,
		KubeContext:       firstNonEmpty(args.Kube.Context, args.commonArgs.Kube.Context),
		KubeconfigPath:    args.Kube.Kubeconfig,
		SessionId:         sessionID,
		Requester:         args.requester(),
	}
	s.mirrorSetMeta(ctx, conn, sessionID, &apiv1.MirrorSessionMeta{
		Command:     "torque.logs.query",
		Requester:   args.requester(),
		KubeContext: req.GetKubeContext(),
		Namespace:   strings.Join(req.GetNamespaces(), ","),
	}, map[string]string{"logs.pod_query": args.PodQuery})
	stream, err := apiv1.NewLogServiceClient(conn).StreamLogs(ctx, req)
	if err != nil {
		s.finishSession(sessionID, err, nil)
		return mapErrorResult("REMOTE_LOGS_FAILED", err, true), nil
	}
	lines := make([]map[string]any, 0, minInt(int(args.TailLines), s.cfg.MaxLogLinesReturned))
	for len(lines) < s.cfg.MaxLogLinesReturned {
		line, err := stream.Recv()
		if recvEOF(err) {
			break
		}
		if err != nil {
			s.finishSession(sessionID, err, map[string]any{"lineCount": len(lines)})
			return mapErrorResult("REMOTE_LOGS_FAILED", err, true), nil
		}
		m := protoMap(line)
		lines = append(lines, m)
		text := strings.TrimSpace(line.GetRendered())
		if text == "" {
			text = strings.TrimSpace(line.GetRaw())
		}
		s.sessions.appendEvent(sessionID, "log.line", text, m)
		if !args.Follow && int64(len(lines)) >= args.TailLines {
			break
		}
	}
	s.finishSession(sessionID, nil, map[string]any{"lineCount": len(lines)})
	return textResult(fmt.Sprintf("Returned %d log lines.", len(lines)), map[string]any{
		"sessionId": sessionID,
		"state":     "succeeded",
		"lineCount": len(lines),
		"lines":     lines,
	}, nilLink(sessionID)...), nil
}

func (s *Server) toolBuildRun(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args buildRunArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	if args.Push || args.Load || args.Sign {
		if err := requireConfirmed(s.cfg.EnableWrite, args.Safety.Confirm, "remote build publish/load/sign"); err != nil {
			return mapErrorResult("CONFIRMATION_REQUIRED", err, false), nil
		}
	}
	ctx, cancel := withTimeout(ctx, args.timeout(s.cfg.DefaultToolTimeoutSeconds))
	defer cancel()
	conn, err := s.dialRemote(ctx)
	if err != nil {
		return mapErrorResult("REMOTE_DIAL_FAILED", err, true), nil
	}
	defer conn.Close()
	rec := s.sessions.create("build", args.requester(), true)
	sessionID := args.sessionID(rec.SessionID)
	s.recordRemoteSession(sessionID, "build", args.requester())
	opts := buildOptionsFromArgs(args)
	opts.CacheFrom, opts.CacheTo, err = buildsvc.ApplyS3Cache(opts.CacheFrom, opts.CacheTo, opts.S3Cache, buildsvc.DefaultS3CacheName(opts.ContextDir, opts.Tags))
	if err != nil {
		s.finishSession(sessionID, err, nil)
		return mapErrorResult("INVALID_ARGUMENTS", err, false), nil
	}
	req := &apiv1.RunBuildRequest{
		SessionId: sessionID,
		Requester: args.requester(),
		Options:   convert.BuildOptionsToProto(opts),
	}
	s.mirrorSetMeta(ctx, conn, sessionID, &apiv1.MirrorSessionMeta{
		Command:   "torque.build.run",
		Requester: args.requester(),
	}, map[string]string{"build.context_dir": opts.ContextDir, "build.dockerfile": opts.Dockerfile})
	stream, err := apiv1.NewBuildServiceClient(conn).RunBuild(ctx, req)
	if err != nil {
		s.finishSession(sessionID, err, nil)
		return mapErrorResult("REMOTE_BUILD_FAILED", err, true), nil
	}
	logs := []map[string]any{}
	var result map[string]any
	for {
		event, err := stream.Recv()
		if recvEOF(err) {
			break
		}
		if err != nil {
			s.finishSession(sessionID, err, map[string]any{"logCount": len(logs)})
			return mapErrorResult("REMOTE_BUILD_FAILED", err, true), nil
		}
		s.sessions.appendEvent(sessionID, "build.event", "", protoMap(event))
		if log := event.GetLog(); log != nil && len(logs) < s.cfg.MaxEventsReturned {
			logs = append(logs, protoMap(log))
		}
		if res := event.GetResult(); res != nil {
			result = protoMap(res)
			if strings.TrimSpace(res.GetError()) != "" {
				err := fmt.Errorf("remote build failed: %s", res.GetError())
				s.finishSession(sessionID, err, result)
				return textToolError("REMOTE_BUILD_FAILED", err.Error(), true, nil, map[string]any{"sessionId": sessionID, "result": result}), nil
			}
		}
	}
	s.finishSession(sessionID, nil, result)
	return textResult("Remote build completed.", map[string]any{"sessionId": sessionID, "state": "succeeded", "logs": logs, "result": result}, nilLink(sessionID)...), nil
}

func (s *Server) toolShipRun(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args shipRunArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	if strings.TrimSpace(args.Chart) == "" || strings.TrimSpace(args.Release) == "" {
		return textToolError("INVALID_ARGUMENTS", "chart and release are required", false, nil, nil), nil
	}
	if !args.PlanOnly {
		if err := requireConfirmed(s.cfg.EnableWrite, args.Safety.Confirm || args.Yes, "ship release"); err != nil {
			return mapErrorResult("CONFIRMATION_REQUIRED", err, false), nil
		}
	}
	cmdArgs := s.shipCommandArgs(args)
	rec := s.sessions.create("ship", args.requester(), strings.TrimSpace(s.cfg.RemoteAgent) != "")
	sessionID := args.sessionID(rec.SessionID)
	s.recordRemoteSession(sessionID, "ship", args.requester())
	res := s.runTorqueCommand(ctx, "torque.ship.run", cmdArgs, args.timeout(s.cfg.DefaultToolTimeoutSeconds))
	safeArgs := redactArgList(cmdArgs)
	safeStdout := s.redactText(res.stdout)
	safeStderr := s.redactText(res.stderr)
	summary := map[string]any{}
	if strings.TrimSpace(args.EvidenceDir) != "" {
		summary = s.redactMap(readOptionalJSONFile(filepath.Join(args.EvidenceDir, "ship.json")))
	}
	structured := map[string]any{
		"sessionId": sessionID,
		"args":      safeArgs,
		"stdout":    safeStdout,
		"stderr":    safeStderr,
		"exitCode":  res.exitCode,
		"summary":   summary,
	}
	s.sessions.appendEvent(sessionID, "ship.command", strings.Join(safeArgs, " "), structured)
	if res.err != nil {
		s.finishSession(sessionID, res.err, structured)
		return textToolError("SHIP_FAILED", res.err.Error(), true, nil, structured), nil
	}
	s.finishSession(sessionID, nil, structured)
	return textResult("Torque ship completed.", structured, nilLink(sessionID)...), nil
}

func (s *Server) toolApplyRun(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args applyRunArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	dryRun := args.DryRun || args.Safety.DryRun
	if !dryRun {
		if err := requireConfirmed(s.cfg.EnableWrite, args.Safety.Confirm, "remote apply"); err != nil {
			return mapErrorResult("CONFIRMATION_REQUIRED", err, false), nil
		}
	}
	wait := true
	if args.HelmWait != nil {
		wait = *args.HelmWait
	}
	atomic := true
	if args.Atomic != nil {
		atomic = *args.Atomic
	}
	timeout := args.TimeoutSeconds
	if timeout <= 0 {
		timeout = 300
	}
	ctx, cancel := withTimeout(ctx, int(timeout)+30)
	defer cancel()
	conn, err := s.dialRemote(ctx)
	if err != nil {
		return mapErrorResult("REMOTE_DIAL_FAILED", err, true), nil
	}
	defer conn.Close()
	rec := s.sessions.create("apply", args.requester(), true)
	sessionID := args.sessionID(rec.SessionID)
	s.recordRemoteSession(sessionID, "apply", args.requester())
	req := &apiv1.DeployApplyRequest{
		SessionId: sessionID,
		Requester: args.requester(),
		Options: &apiv1.DeployApplyOptions{
			Release:         args.Release,
			Chart:           args.Chart,
			Namespace:       firstNonEmpty(args.Namespace, args.Kube.Namespace),
			Version:         args.Version,
			ValuesFiles:     args.ValuesFiles,
			SetValues:       args.Set,
			SetStringValues: args.SetString,
			SetFileValues:   args.SetFile,
			TimeoutSeconds:  timeout,
			Wait:            wait,
			Atomic:          atomic,
			UpgradeOnly:     args.UpgradeOnly,
			CreateNamespace: args.CreateNamespace,
			DryRun:          dryRun,
			Diff:            args.Diff,
			KubeContext:     args.Kube.Context,
			KubeconfigPath:  args.Kube.Kubeconfig,
		},
	}
	s.mirrorSetMeta(ctx, conn, sessionID, &apiv1.MirrorSessionMeta{
		Command:     "torque.apply.run",
		Requester:   args.requester(),
		KubeContext: args.Kube.Context,
		Namespace:   req.Options.Namespace,
		Release:     args.Release,
		Chart:       args.Chart,
	}, nil)
	stream, err := apiv1.NewDeployServiceClient(conn).Apply(ctx, req)
	if err != nil {
		s.finishSession(sessionID, err, nil)
		return mapErrorResult("REMOTE_APPLY_FAILED", err, true), nil
	}
	events, derr := s.consumeDeployEvents(sessionID, stream)
	s.finishSession(sessionID, derr, map[string]any{"eventCount": len(events), "dryRun": dryRun})
	if derr != nil {
		return mapErrorResult("REMOTE_APPLY_FAILED", derr, true), nil
	}
	return textResult("Remote apply completed.", map[string]any{"sessionId": sessionID, "state": "succeeded", "events": events}, nilLink(sessionID)...), nil
}

func (s *Server) toolDeleteRun(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args deleteRunArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	dryRun := args.DryRun || args.Safety.DryRun
	if !dryRun {
		if err := requireConfirmed(s.cfg.EnableWrite, args.Safety.Confirm, "remote delete"); err != nil {
			return mapErrorResult("CONFIRMATION_REQUIRED", err, false), nil
		}
	}
	wait := true
	if args.HelmWait != nil {
		wait = *args.HelmWait
	}
	timeout := args.TimeoutSeconds
	if timeout <= 0 {
		timeout = 300
	}
	ctx, cancel := withTimeout(ctx, int(timeout)+30)
	defer cancel()
	conn, err := s.dialRemote(ctx)
	if err != nil {
		return mapErrorResult("REMOTE_DIAL_FAILED", err, true), nil
	}
	defer conn.Close()
	rec := s.sessions.create("delete", args.requester(), true)
	sessionID := args.sessionID(rec.SessionID)
	s.recordRemoteSession(sessionID, "delete", args.requester())
	req := &apiv1.DeployDestroyRequest{
		SessionId: sessionID,
		Requester: args.requester(),
		Options: &apiv1.DeployDestroyOptions{
			Release:        args.Release,
			Namespace:      firstNonEmpty(args.Namespace, args.Kube.Namespace),
			TimeoutSeconds: timeout,
			Wait:           wait,
			KeepHistory:    args.KeepHistory,
			DryRun:         dryRun,
			Force:          args.Force,
			DisableHooks:   args.DisableHooks,
			KubeContext:    args.Kube.Context,
			KubeconfigPath: args.Kube.Kubeconfig,
		},
	}
	s.mirrorSetMeta(ctx, conn, sessionID, &apiv1.MirrorSessionMeta{
		Command:     "torque.delete.run",
		Requester:   args.requester(),
		KubeContext: args.Kube.Context,
		Namespace:   req.Options.Namespace,
		Release:     args.Release,
	}, nil)
	stream, err := apiv1.NewDeployServiceClient(conn).Destroy(ctx, req)
	if err != nil {
		s.finishSession(sessionID, err, nil)
		return mapErrorResult("REMOTE_DELETE_FAILED", err, true), nil
	}
	events, derr := s.consumeDeployEvents(sessionID, stream)
	s.finishSession(sessionID, derr, map[string]any{"eventCount": len(events), "dryRun": dryRun})
	if derr != nil {
		return mapErrorResult("REMOTE_DELETE_FAILED", derr, true), nil
	}
	return textResult("Remote delete completed.", map[string]any{"sessionId": sessionID, "state": "succeeded", "events": events}, nilLink(sessionID)...), nil
}

func (s *Server) consumeDeployEvents(sessionID string, stream interface {
	Recv() (*apiv1.DeployEvent, error)
}) ([]map[string]any, error) {
	events := []map[string]any{}
	for {
		evt, err := stream.Recv()
		if recvEOF(err) {
			break
		}
		if err != nil {
			return events, err
		}
		m := protoMap(evt)
		if len(events) < s.cfg.MaxEventsReturned {
			events = append(events, m)
		}
		message := strings.TrimSpace(evt.GetJson())
		s.sessions.appendEvent(sessionID, "deploy.event", message, m)
	}
	return events, nil
}

func (s *Server) toolVerifyChart(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args verifyArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	args.Target = "chart"
	return s.runRemoteVerify(ctx, args)
}

func (s *Server) toolVerifyNamespace(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args verifyArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	args.Target = "namespace"
	return s.runRemoteVerify(ctx, args)
}

func (s *Server) runRemoteVerify(ctx context.Context, args verifyArgs) (toolResult, error) {
	ctx, cancel := withTimeout(ctx, args.timeout(s.cfg.DefaultToolTimeoutSeconds))
	defer cancel()
	conn, err := s.dialRemote(ctx)
	if err != nil {
		return mapErrorResult("REMOTE_DIAL_FAILED", err, true), nil
	}
	defer conn.Close()
	rec := s.sessions.create("verify", args.requester(), true)
	sessionID := args.sessionID(rec.SessionID)
	s.recordRemoteSession(sessionID, "verify", args.requester())
	opts := &apiv1.VerifyOptions{
		Mode:       firstNonEmpty(args.Mode, "warn"),
		FailOn:     firstNonEmpty(args.FailOn, "high"),
		Format:     firstNonEmpty(args.Format, "json"),
		RulesDir:   args.RulesDir,
		Policy:     args.Policy,
		PolicyMode: args.PolicyMode,
		Baseline:   args.Baseline,
	}
	req := &apiv1.VerifyRequest{Options: opts, SessionId: sessionID, Requester: args.requester()}
	if strings.EqualFold(args.Target, "namespace") {
		ns := firstNonEmpty(args.Namespace, args.Kube.Namespace)
		req.Target = &apiv1.VerifyRequest_Namespace{Namespace: &apiv1.VerifyNamespaceOptions{
			Namespace:      ns,
			KubeContext:    args.Kube.Context,
			KubeconfigPath: args.Kube.Kubeconfig,
		}}
	} else {
		req.Target = &apiv1.VerifyRequest_Chart{Chart: &apiv1.VerifyChartOptions{
			Chart:           args.Chart,
			Release:         args.Release,
			Namespace:       firstNonEmpty(args.Namespace, args.Kube.Namespace),
			Version:         args.Version,
			ValuesFiles:     args.ValuesFiles,
			SetValues:       args.Set,
			SetStringValues: args.SetString,
			SetFileValues:   args.SetFile,
			KubeContext:     args.Kube.Context,
			KubeconfigPath:  args.Kube.Kubeconfig,
		}}
	}
	stream, err := apiv1.NewVerifyServiceClient(conn).Verify(ctx, req)
	if err != nil {
		s.finishSession(sessionID, err, nil)
		return mapErrorResult("REMOTE_VERIFY_FAILED", err, true), nil
	}
	events := []map[string]any{}
	var done map[string]any
	for {
		ev, err := stream.Recv()
		if recvEOF(err) {
			break
		}
		if err != nil {
			s.finishSession(sessionID, err, map[string]any{"eventCount": len(events)})
			return mapErrorResult("REMOTE_VERIFY_FAILED", err, true), nil
		}
		m := protoMap(ev)
		if len(events) < s.cfg.MaxEventsReturned {
			events = append(events, m)
		}
		s.sessions.appendEvent(sessionID, "verify.event", "", m)
		if ev.GetDone() != nil {
			done = protoMap(ev.GetDone())
		}
	}
	s.finishSession(sessionID, nil, map[string]any{"eventCount": len(events), "done": done})
	return textResult("Remote verify completed.", map[string]any{"sessionId": sessionID, "state": "succeeded", "done": done, "events": events}, nilLink(sessionID)...), nil
}

func (s *Server) toolCaptureSummarize(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args struct {
		Path      string `json:"path"`
		SessionID string `json:"sessionId"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	sum, err := capture.Summarize(ctx, args.Path, capture.SummaryOptions{SessionID: args.SessionID})
	if err != nil {
		return mapErrorResult("CAPTURE_SUMMARY_FAILED", err, false), nil
	}
	return textResult("Capture summary returned.", sum), nil
}

func (s *Server) toolCaptureReadArtifact(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args struct {
		Path  string   `json:"path"`
		Names []string `json:"names"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	arts, err := capture.ReadArtifacts(ctx, args.Path, args.Names...)
	if err != nil {
		return mapErrorResult("CAPTURE_ARTIFACT_FAILED", err, false), nil
	}
	return textResult("Capture artifacts returned.", map[string]any{"artifacts": arts}), nil
}

func (s *Server) toolApplyPlan(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args struct {
		Chart       string   `json:"chart"`
		Release     string   `json:"release"`
		Namespace   string   `json:"namespace"`
		Version     string   `json:"version"`
		ValuesFiles []string `json:"valuesFiles"`
		Set         []string `json:"set"`
		SetString   []string `json:"setString"`
		SetFile     []string `json:"setFile"`
		Kube        struct {
			Context    string `json:"context"`
			Kubeconfig string `json:"kubeconfig"`
		} `json:"kube"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	cmdArgs := []string{"apply", "plan", "--format", "json", "--chart", args.Chart, "--release", args.Release}
	if args.Namespace != "" {
		cmdArgs = append(cmdArgs, "--namespace", args.Namespace)
	}
	if args.Version != "" {
		cmdArgs = append(cmdArgs, "--version", args.Version)
	}
	for _, v := range args.ValuesFiles {
		cmdArgs = append(cmdArgs, "--values", v)
	}
	for _, v := range args.Set {
		cmdArgs = append(cmdArgs, "--set", v)
	}
	for _, v := range args.SetString {
		cmdArgs = append(cmdArgs, "--set-string", v)
	}
	for _, v := range args.SetFile {
		cmdArgs = append(cmdArgs, "--set-file", v)
	}
	if args.Kube.Context != "" {
		cmdArgs = append(cmdArgs, "--context", args.Kube.Context)
	}
	if args.Kube.Kubeconfig != "" {
		cmdArgs = append(cmdArgs, "--kubeconfig", args.Kube.Kubeconfig)
	}
	return s.runTorqueJSONTool(ctx, "torque.apply.plan", cmdArgs)
}

func (s *Server) toolStackPlan(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args stackArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	if strings.TrimSpace(s.cfg.RemoteAgent) != "" {
		ctx, cancel := withTimeout(ctx, args.timeout(s.cfg.DefaultToolTimeoutSeconds))
		defer cancel()
		conn, err := s.dialRemote(ctx)
		if err != nil {
			return mapErrorResult("REMOTE_DIAL_FAILED", err, true), nil
		}
		defer conn.Close()
		rec := s.sessions.create("stack.plan", args.requester(), true)
		sessionID := args.sessionID(rec.SessionID)
		s.recordRemoteSession(sessionID, "stack.plan", args.requester())
		resp, err := apiv1.NewStackServiceClient(conn).Plan(ctx, &apiv1.StackPlanRequest{
			Selector:  stackSelectorFromMCPArgs(args),
			SessionId: sessionID,
			Requester: args.requester(),
		})
		if err != nil {
			s.finishSession(sessionID, err, nil)
			return mapErrorResult("REMOTE_STACK_PLAN_FAILED", err, true), nil
		}
		parsed := parseJSONString(resp.GetJson())
		s.sessions.appendEvent(sessionID, "stack.plan", "compiled stack plan", map[string]any{"nodeCount": resp.GetNodeCount(), "stackName": resp.GetStackName()})
		s.finishSession(sessionID, nil, map[string]any{"nodeCount": resp.GetNodeCount(), "stackName": resp.GetStackName()})
		return textResult("Remote stack plan compiled.", map[string]any{
			"sessionId": sessionID,
			"state":     "succeeded",
			"nodeCount": resp.GetNodeCount(),
			"stackName": resp.GetStackName(),
			"profile":   resp.GetProfile(),
			"plan":      parsed,
		}, nilLink(sessionID)...), nil
	}
	cmdArgs := []string{"stack", "plan", "--output", "json"}
	if args.Config != "" {
		cmdArgs = append(cmdArgs, "--config", args.Config)
	} else if args.Root != "" {
		cmdArgs = append(cmdArgs, "--root", args.Root)
	}
	if args.Profile != "" {
		cmdArgs = append(cmdArgs, "--profile", args.Profile)
	}
	for _, v := range args.Clusters {
		cmdArgs = append(cmdArgs, "--cluster", v)
	}
	for _, v := range args.Tags {
		cmdArgs = append(cmdArgs, "--tag", v)
	}
	for _, v := range args.FromPaths {
		cmdArgs = append(cmdArgs, "--from-path", v)
	}
	for _, v := range args.Releases {
		cmdArgs = append(cmdArgs, "--release", v)
	}
	if args.GitRange != "" {
		cmdArgs = append(cmdArgs, "--git-range", args.GitRange)
	}
	if args.GitIncludeDeps != nil && *args.GitIncludeDeps {
		cmdArgs = append(cmdArgs, "--git-include-deps")
	}
	if args.GitIncludeDependents != nil && *args.GitIncludeDependents {
		cmdArgs = append(cmdArgs, "--git-include-dependents")
	}
	if args.IncludeDeps != nil && *args.IncludeDeps {
		cmdArgs = append(cmdArgs, "--include-deps")
	}
	if args.IncludeDependents != nil && *args.IncludeDependents {
		cmdArgs = append(cmdArgs, "--include-dependents")
	}
	if args.AllowMissingDeps != nil && *args.AllowMissingDeps {
		cmdArgs = append(cmdArgs, "--allow-missing-deps")
	}
	if args.InferDeps != nil && !*args.InferDeps {
		cmdArgs = append(cmdArgs, "--infer-deps=false")
	}
	if args.InferConfigRefs != nil && *args.InferConfigRefs {
		cmdArgs = append(cmdArgs, "--infer-config-refs")
	}
	if args.Bundle != "" {
		cmdArgs = append(cmdArgs, "--bundle", args.Bundle)
	}
	return s.runTorqueJSONTool(ctx, "torque.stack.plan", cmdArgs)
}

func (s *Server) toolStackApply(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args stackArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	dryRun := args.DryRun || args.Safety.DryRun
	if !dryRun {
		if err := requireConfirmed(s.cfg.EnableWrite, args.Safety.Confirm, "remote stack apply"); err != nil {
			return mapErrorResult("CONFIRMATION_REQUIRED", err, false), nil
		}
	}
	return s.runRemoteStack(ctx, "apply", args)
}

func (s *Server) toolStackDelete(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args stackArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	if err := requireConfirmed(s.cfg.EnableWrite, args.Safety.Confirm, "remote stack delete"); err != nil {
		return mapErrorResult("CONFIRMATION_REQUIRED", err, false), nil
	}
	return s.runRemoteStack(ctx, "delete", args)
}

func (s *Server) toolStackStatus(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args stackArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	format := strings.ToLower(strings.TrimSpace(args.Format))
	if format == "" {
		format = "json"
	}
	if strings.TrimSpace(s.cfg.RemoteAgent) != "" {
		ctx, cancel := withTimeout(ctx, args.timeout(s.cfg.DefaultToolTimeoutSeconds))
		defer cancel()
		conn, err := s.dialRemote(ctx)
		if err != nil {
			return mapErrorResult("REMOTE_DIAL_FAILED", err, true), nil
		}
		defer conn.Close()
		resp, err := apiv1.NewStackServiceClient(conn).Status(ctx, &apiv1.StackStatusRequest{
			ConfigPath: args.Config,
			Root:       args.Root,
			RunId:      args.RunID,
			Limit:      int32(args.Limit),
			Format:     format,
			Requester:  args.requester(),
		})
		if err != nil {
			return mapErrorResult("REMOTE_STACK_STATUS_FAILED", err, true), nil
		}
		return textResult("Remote stack status returned.", map[string]any{
			"runId":  resp.GetRunId(),
			"status": parseJSONString(resp.GetJson()),
		}), nil
	}
	cmdArgs := []string{"stack", "status", "--format", "json"}
	if args.Config != "" {
		cmdArgs = append(cmdArgs, "--config", args.Config)
	} else if args.Root != "" {
		cmdArgs = append(cmdArgs, "--root", args.Root)
	}
	if args.RunID != "" {
		cmdArgs = append(cmdArgs, "--run-id", args.RunID)
	}
	if args.Limit > 0 {
		cmdArgs = append(cmdArgs, "--tail", fmt.Sprintf("%d", args.Limit))
	}
	return s.runTorqueJSONTool(ctx, "torque.stack.status", cmdArgs)
}

func (s *Server) runRemoteStack(ctx context.Context, command string, args stackArgs) (toolResult, error) {
	if strings.TrimSpace(s.cfg.RemoteAgent) == "" {
		return textToolError("REMOTE_AGENT_REQUIRED", "stack apply/delete requires torque-mcp --remote-agent", false, nil, nil), nil
	}
	ctx, cancel := withTimeout(ctx, args.timeout(s.cfg.DefaultToolTimeoutSeconds))
	defer cancel()
	conn, err := s.dialRemote(ctx)
	if err != nil {
		return mapErrorResult("REMOTE_DIAL_FAILED", err, true), nil
	}
	defer conn.Close()
	rec := s.sessions.create("stack."+command, args.requester(), true)
	sessionID := args.sessionID(rec.SessionID)
	s.recordRemoteSession(sessionID, "stack."+command, args.requester())
	req := &apiv1.StackRunRequest{
		Command:   command,
		Selector:  stackSelectorFromMCPArgs(args),
		Options:   stackRunOptionsFromMCPArgs(args),
		SessionId: sessionID,
		Requester: args.requester(),
	}
	var stream interface {
		Recv() (*apiv1.StackEvent, error)
	}
	if command == "delete" {
		stream, err = apiv1.NewStackServiceClient(conn).Delete(ctx, req)
	} else {
		stream, err = apiv1.NewStackServiceClient(conn).Apply(ctx, req)
	}
	if err != nil {
		s.finishSession(sessionID, err, nil)
		return mapErrorResult("REMOTE_STACK_"+strings.ToUpper(command)+"_FAILED", err, true), nil
	}
	events, runID, derr := s.consumeStackEvents(sessionID, stream)
	s.finishSession(sessionID, derr, map[string]any{"eventCount": len(events), "runId": runID})
	if derr != nil {
		return mapErrorResult("REMOTE_STACK_"+strings.ToUpper(command)+"_FAILED", derr, true), nil
	}
	return textResult("Remote stack "+command+" completed.", map[string]any{
		"sessionId":  sessionID,
		"state":      "succeeded",
		"runId":      runID,
		"eventCount": len(events),
		"events":     events,
	}, nilLink(sessionID)...), nil
}

func (s *Server) consumeStackEvents(sessionID string, stream interface {
	Recv() (*apiv1.StackEvent, error)
}) ([]map[string]any, string, error) {
	events := []map[string]any{}
	runID := ""
	for {
		ev, err := stream.Recv()
		if recvEOF(err) {
			break
		}
		if err != nil {
			return events, runID, err
		}
		if strings.TrimSpace(ev.GetRunId()) != "" {
			runID = strings.TrimSpace(ev.GetRunId())
		}
		m := protoMap(ev)
		if len(events) < s.cfg.MaxEventsReturned {
			events = append(events, m)
		}
		s.sessions.appendEvent(sessionID, "stack.event", strings.TrimSpace(ev.GetJson()), m)
	}
	return events, runID, nil
}

func stackSelectorFromMCPArgs(args stackArgs) *apiv1.StackSelector {
	out := &apiv1.StackSelector{
		ConfigPath:     args.Config,
		Root:           args.Root,
		Profile:        args.Profile,
		Clusters:       args.Clusters,
		Tags:           args.Tags,
		FromPaths:      args.FromPaths,
		Releases:       args.Releases,
		GitRange:       args.GitRange,
		SecretProvider: args.SecretProvider,
		SecretConfig:   args.SecretConfig,
		KubeContext:    args.Kube.Context,
		KubeconfigPath: args.Kube.Kubeconfig,
	}
	out.GitIncludeDeps = cloneBoolPtr(args.GitIncludeDeps)
	out.GitIncludeDependents = cloneBoolPtr(args.GitIncludeDependents)
	out.IncludeDeps = cloneBoolPtr(args.IncludeDeps)
	out.IncludeDependents = cloneBoolPtr(args.IncludeDependents)
	out.AllowMissingDeps = cloneBoolPtr(args.AllowMissingDeps)
	out.InferDeps = cloneBoolPtr(args.InferDeps)
	out.InferConfigRefs = cloneBoolPtr(args.InferConfigRefs)
	return out
}

func stackRunOptionsFromMCPArgs(args stackArgs) *apiv1.StackRunOptions {
	out := &apiv1.StackRunOptions{
		Concurrency:                int32(args.Concurrency),
		Yes:                        boolPtrIfTrue(args.Yes || args.Safety.Confirm),
		DryRun:                     boolPtrIfTrue(args.DryRun || args.Safety.DryRun),
		Diff:                       boolPtrIfTrue(args.Diff),
		CacheApply:                 boolPtrIfTrue(args.CacheApply),
		HelmLogs:                   boolPtrIfTrue(args.HelmLogs),
		RunId:                      args.RunID,
		Retry:                      int32(args.Retry),
		KubeQps:                    args.KubeQPS,
		KubeBurst:                  int32(args.KubeBurst),
		MaxParallelPerNamespace:    int32(args.MaxParallelPerNamespace),
		MaxParallelKind:            args.MaxParallelKind,
		ParallelismGroupLimit:      int32(args.ParallelismGroupLimit),
		Lock:                       cloneBoolPtr(args.Lock),
		TakeoverLock:               cloneBoolPtr(args.TakeoverLock),
		LockTtlSeconds:             args.LockTTLSeconds,
		LockOwner:                  args.LockOwner,
		FailMode:                   args.FailMode,
		AdaptiveMin:                int32(args.AdaptiveMin),
		AdaptiveWindow:             int32(args.AdaptiveWindow),
		AdaptiveRampSuccesses:      int32(args.AdaptiveRampSuccesses),
		AdaptiveRampMaxFailureRate: args.AdaptiveRampMaxFailureRate,
		AdaptiveCooldownSevere:     int32(args.AdaptiveCooldownSevere),
	}
	out.ProgressiveConcurrency = cloneBoolPtr(args.ProgressiveConcurrency)
	out.FailFast = cloneBoolPtr(args.FailFast)
	out.ContinueOnError = cloneBoolPtr(args.ContinueOnError)
	return out
}

func (s *Server) runTorqueJSONTool(ctx context.Context, toolName string, args []string) (toolResult, error) {
	res := s.runTorqueCommand(ctx, toolName, args, s.cfg.DefaultToolTimeoutSeconds)
	if res.err != nil {
		return textToolError("TORQUE_COMMAND_FAILED", fmt.Sprintf("%s failed: %v\n%s", toolName, res.err, res.stderr), true, nil, map[string]any{"args": args}), nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(res.stdout), &parsed); err != nil {
		parsed = map[string]any{"stdout": res.stdout, "stderr": res.stderr}
	}
	return textResult(toolName+" completed.", parsed), nil
}

type torqueCommandResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func (s *Server) runTorqueCommand(ctx context.Context, toolName string, args []string, timeoutSeconds int) torqueCommandResult {
	bin := firstNonEmpty(os.Getenv("TORQUE_MCP_TORQUE_BIN"), "torque")
	ctx, cancel := withTimeout(ctx, timeoutSeconds)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = s.torqueCommandEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}
	return torqueCommandResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode, err: err}
}

func (s *Server) shipCommandArgs(args shipRunArgs) []string {
	cmdArgs := s.remoteTorqueArgs()
	cmdArgs = append(cmdArgs, "ship", "--chart", args.Chart, "--release", args.Release)
	if args.Namespace != "" {
		cmdArgs = append(cmdArgs, "--namespace", args.Namespace)
	} else if args.Kube.Namespace != "" {
		cmdArgs = append(cmdArgs, "--namespace", args.Kube.Namespace)
	}
	if args.Version != "" {
		cmdArgs = append(cmdArgs, "--version", args.Version)
	}
	for _, v := range args.ValuesFiles {
		cmdArgs = append(cmdArgs, "--values", v)
	}
	for _, v := range args.Set {
		cmdArgs = append(cmdArgs, "--set", v)
	}
	for _, v := range args.SetString {
		cmdArgs = append(cmdArgs, "--set-string", v)
	}
	for _, v := range args.SetFile {
		cmdArgs = append(cmdArgs, "--set-file", v)
	}
	if args.SecretProvider != "" {
		cmdArgs = append(cmdArgs, "--secret-provider", args.SecretProvider)
	}
	if args.SecretConfig != "" {
		cmdArgs = append(cmdArgs, "--secret-config", args.SecretConfig)
	}
	if args.BuildContext != "" {
		cmdArgs = append(cmdArgs, "--build", args.BuildContext)
	}
	if args.Dockerfile != "" {
		cmdArgs = append(cmdArgs, "--dockerfile", args.Dockerfile)
	}
	for _, tag := range args.Tags {
		cmdArgs = append(cmdArgs, "--tag", tag)
	}
	for _, platform := range args.Platforms {
		cmdArgs = append(cmdArgs, "--platform", platform)
	}
	for _, v := range args.BuildArgs {
		cmdArgs = append(cmdArgs, "--build-arg", v)
	}
	for _, v := range args.BuildSecrets {
		cmdArgs = append(cmdArgs, "--build-secret", v)
	}
	for _, v := range args.CacheFrom {
		cmdArgs = append(cmdArgs, "--cache-from", v)
	}
	for _, v := range args.CacheTo {
		cmdArgs = append(cmdArgs, "--cache-to", v)
	}
	if args.S3Cache != "" {
		cmdArgs = append(cmdArgs, "--s3-cache", args.S3Cache)
	}
	if args.S3CacheRegion != "" {
		cmdArgs = append(cmdArgs, "--s3-cache-region", args.S3CacheRegion)
	}
	if args.S3CacheName != "" {
		cmdArgs = append(cmdArgs, "--s3-cache-name", args.S3CacheName)
	}
	if args.S3CacheMode != "" {
		cmdArgs = append(cmdArgs, "--s3-cache-mode", args.S3CacheMode)
	}
	if args.S3CacheEndpoint != "" {
		cmdArgs = append(cmdArgs, "--s3-cache-endpoint-url", args.S3CacheEndpoint)
	}
	if args.S3CachePathStyle {
		cmdArgs = append(cmdArgs, "--s3-cache-path-style")
	}
	if args.Push != nil {
		cmdArgs = append(cmdArgs, "--push="+fmt.Sprintf("%t", *args.Push))
	}
	if args.Load {
		cmdArgs = append(cmdArgs, "--load")
	}
	if args.NoCache {
		cmdArgs = append(cmdArgs, "--no-cache")
	}
	if args.Attest != nil {
		cmdArgs = append(cmdArgs, "--attest="+fmt.Sprintf("%t", *args.Attest))
	}
	if args.SBOM {
		cmdArgs = append(cmdArgs, "--sbom")
	}
	if args.Provenance {
		cmdArgs = append(cmdArgs, "--provenance")
	}
	if args.AttestDir != "" {
		cmdArgs = append(cmdArgs, "--attest-dir", args.AttestDir)
	}
	if args.Hermetic {
		cmdArgs = append(cmdArgs, "--hermetic")
	}
	if args.AllowNetwork {
		cmdArgs = append(cmdArgs, "--allow-network")
	}
	if args.AllowUnpinned {
		cmdArgs = append(cmdArgs, "--allow-unpinned-bases")
	}
	if args.BuildPolicy != "" {
		cmdArgs = append(cmdArgs, "--build-policy", args.BuildPolicy)
	}
	if args.BuildPolicyMode != "" {
		cmdArgs = append(cmdArgs, "--build-policy-mode", args.BuildPolicyMode)
	}
	if args.Builder != "" {
		cmdArgs = append(cmdArgs, "--builder", args.Builder)
	}
	if args.AuthFile != "" {
		cmdArgs = append(cmdArgs, "--authfile", args.AuthFile)
	}
	if args.Sandbox {
		cmdArgs = append(cmdArgs, "--sandbox")
	}
	if args.SandboxConfig != "" {
		cmdArgs = append(cmdArgs, "--sandbox-config", args.SandboxConfig)
	}
	if args.BuildOutput != "" {
		cmdArgs = append(cmdArgs, "--build-output", args.BuildOutput)
	}
	if args.VerifyMode != "" {
		cmdArgs = append(cmdArgs, "--verify-mode", args.VerifyMode)
	}
	if args.VerifyFailOn != "" {
		cmdArgs = append(cmdArgs, "--verify-fail-on", args.VerifyFailOn)
	}
	if args.EvidenceDir != "" {
		cmdArgs = append(cmdArgs, "--evidence-dir", args.EvidenceDir)
	}
	for _, tag := range args.CaptureTags {
		cmdArgs = append(cmdArgs, "--capture-tag", tag)
	}
	if args.NoCapture {
		cmdArgs = append(cmdArgs, "--no-capture")
	}
	if args.CreateNamespace {
		cmdArgs = append(cmdArgs, "--create-namespace")
	}
	if args.HelmWait != nil {
		cmdArgs = append(cmdArgs, "--wait="+fmt.Sprintf("%t", *args.HelmWait))
	}
	if args.Atomic != nil {
		cmdArgs = append(cmdArgs, "--atomic="+fmt.Sprintf("%t", *args.Atomic))
	}
	if args.Yes || args.Safety.Confirm {
		cmdArgs = append(cmdArgs, "--yes")
	}
	if args.Safety.NonInteractive {
		cmdArgs = append(cmdArgs, "--non-interactive")
	}
	if args.TimeoutSeconds > 0 {
		cmdArgs = append(cmdArgs, "--timeout", fmt.Sprintf("%ds", args.TimeoutSeconds))
	}
	if args.PlanOnly || args.Safety.DryRun {
		cmdArgs = append(cmdArgs, "--plan-only")
	}
	if args.SkipBuild {
		cmdArgs = append(cmdArgs, "--skip-build")
	}
	if args.SkipVerify {
		cmdArgs = append(cmdArgs, "--skip-verify")
	}
	if args.SkipExplain {
		cmdArgs = append(cmdArgs, "--skip-explain")
	}
	if args.Kube.Context != "" {
		cmdArgs = append(cmdArgs, "--context", args.Kube.Context)
	}
	if args.Kube.Kubeconfig != "" {
		cmdArgs = append(cmdArgs, "--kubeconfig", args.Kube.Kubeconfig)
	}
	return cmdArgs
}

func (s *Server) remoteTorqueArgs() []string {
	var args []string
	if strings.TrimSpace(s.cfg.RemoteAgent) != "" {
		args = append(args, "--remote-agent", strings.TrimSpace(s.cfg.RemoteAgent))
	}
	if s.cfg.RemoteTLS {
		args = append(args, "--remote-tls")
	}
	if strings.TrimSpace(s.cfg.RemoteTLSCA) != "" {
		args = append(args, "--remote-tls-ca", strings.TrimSpace(s.cfg.RemoteTLSCA))
	}
	if s.cfg.RemoteTLSInsecureSkipVerify {
		args = append(args, "--remote-tls-insecure-skip-verify")
	}
	if strings.TrimSpace(s.cfg.RemoteTLSServerName) != "" {
		args = append(args, "--remote-tls-server-name", strings.TrimSpace(s.cfg.RemoteTLSServerName))
	}
	if strings.TrimSpace(s.cfg.RemoteTLSClientCert) != "" {
		args = append(args, "--remote-tls-client-cert", strings.TrimSpace(s.cfg.RemoteTLSClientCert))
	}
	if strings.TrimSpace(s.cfg.RemoteTLSClientKey) != "" {
		args = append(args, "--remote-tls-client-key", strings.TrimSpace(s.cfg.RemoteTLSClientKey))
	}
	return args
}

func (s *Server) torqueCommandEnv() []string {
	env := os.Environ()
	set := func(key string, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		prefix := key + "="
		for i, kv := range env {
			if strings.HasPrefix(kv, prefix) {
				env[i] = prefix + value
				return
			}
		}
		env = append(env, prefix+value)
	}
	set("TORQUE_REMOTE_TOKEN", s.cfg.RemoteToken)
	if s.cfg.RemoteTLS {
		set("TORQUE_REMOTE_TLS", "1")
	}
	set("TORQUE_REMOTE_TLS_CA", s.cfg.RemoteTLSCA)
	set("TORQUE_REMOTE_TLS_SERVER_NAME", s.cfg.RemoteTLSServerName)
	if s.cfg.RemoteTLSInsecureSkipVerify {
		set("TORQUE_REMOTE_TLS_INSECURE_SKIP_VERIFY", "1")
	}
	set("TORQUE_REMOTE_TLS_CLIENT_CERT", s.cfg.RemoteTLSClientCert)
	set("TORQUE_REMOTE_TLS_CLIENT_KEY", s.cfg.RemoteTLSClientKey)
	return env
}

func redactArgList(args []string) []string {
	out := make([]string, len(args))
	redactNext := false
	for i, arg := range args {
		if redactNext {
			out[i] = "<redacted>"
			redactNext = false
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name, val, hasVal := strings.Cut(arg, "=")
			if sensitiveArgName(name) {
				if hasVal {
					out[i] = name + "=<redacted>"
				} else {
					out[i] = name
					redactNext = true
				}
				continue
			}
			if hasVal && looksSensitiveValue(val) {
				out[i] = name + "=<redacted>"
				continue
			}
		}
		if looksSensitiveValue(arg) {
			out[i] = "<redacted>"
			continue
		}
		out[i] = arg
	}
	return out
}

func sensitiveArgName(name string) bool {
	upper := strings.ToUpper(strings.TrimLeft(name, "-"))
	for _, needle := range []string{"TOKEN", "PASSWORD", "SECRET", "KEY", "AUTH"} {
		if strings.Contains(upper, needle) {
			return true
		}
	}
	return false
}

func looksSensitiveValue(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "password=") ||
		strings.Contains(lower, "passwd=") ||
		strings.Contains(lower, "token=") ||
		strings.Contains(lower, "secret=") ||
		strings.Contains(lower, "apikey=") ||
		strings.Contains(lower, "api_key=")
}

func (s *Server) redactText(text string) string {
	out := text
	for _, secret := range []string{s.cfg.RemoteToken} {
		secret = strings.TrimSpace(secret)
		if secret != "" {
			out = strings.ReplaceAll(out, secret, "<redacted>")
		}
	}
	return out
}

func (s *Server) redactMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return in
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(s.redactText(string(raw))), &out); err != nil {
		return in
	}
	return out
}

func readOptionalJSONFile(path string) map[string]any {
	raw, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"raw": string(raw)}
	}
	return out
}

func (s *Server) toolEnvInspect(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		key, val, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(key, "TORQUE_") || strings.HasPrefix(key, "KTL_") || key == "KUBECONFIG" {
			if looksSensitive(key) {
				val = "<redacted>"
			}
			env[key] = val
		}
	}
	return textResult("Torque environment returned.", map[string]any{
		"env": env,
		"policy": map[string]any{
			"writeEnabled":        s.cfg.EnableWrite,
			"remoteAgent":         s.cfg.RemoteAgent,
			"maxEventsReturned":   s.cfg.MaxEventsReturned,
			"maxLogLinesReturned": s.cfg.MaxLogLinesReturned,
		},
	}), nil
}

func nilLink(sessionID string) []contentItem {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	return []contentItem{resourceLink("torque://sessions/"+strings.TrimSpace(sessionID), "session", "Torque session", "application/json")}
}

func sortedToolNames(tools map[string]toolSpec) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

func looksSensitive(key string) bool {
	upper := strings.ToUpper(key)
	for _, needle := range []string{"TOKEN", "PASSWORD", "SECRET", "KEY", "AUTH"} {
		if strings.Contains(upper, needle) {
			return true
		}
	}
	return false
}

func decodeReaderJSON(r io.Reader, out any) error {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	return dec.Decode(out)
}

func parseJSONString(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return map[string]any{"raw": raw}
	}
	return parsed
}

func cloneBoolPtr(in *bool) *bool {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func boolPtrIfTrue(v bool) *bool {
	if !v {
		return nil
	}
	return &v
}
