package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ingresslabs/torque/internal/deploy"
	"github.com/ingresslabs/torque/internal/secretstore"
	"github.com/ingresslabs/torque/internal/stack"
	apiv1 "github.com/ingresslabs/torque/pkg/api/torque/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type StackServer struct {
	apiv1.UnimplementedStackServiceServer
	Mirror *MirrorServer
}

type stackCompileResult struct {
	Plan             *stack.Plan
	Root             string
	Profile          string
	Selector         stack.Selector
	Clusters         []string
	SecretProvider   string
	SecretConfig     string
	KubeContext      string
	KubeconfigPath   string
	ApplyDefaults    stackRunDefaults
	DeleteDefaults   stackRunDefaults
	Progressive      bool
	AdaptiveOverride *stack.AdaptiveConcurrencyOptions
}

type stackRunDefaults struct {
	DryRun      *bool
	Diff        *bool
	FailFast    *bool
	Retry       *int
	Lock        *bool
	Takeover    *bool
	LockTTL     *time.Duration
	LockOwner   *string
	ConfirmSize *int
}

func (s *StackServer) Plan(ctx context.Context, req *apiv1.StackPlanRequest) (*apiv1.StackPlanResult, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	compiled, err := compileStackForAgent(ctx, req.GetSelector())
	if err != nil {
		return nil, stackStatusError("compile stack", err)
	}
	raw, err := json.MarshalIndent(compiled.Plan, "", "  ")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal stack plan: %v", err)
	}
	if s.Mirror != nil && strings.TrimSpace(req.GetSessionId()) != "" {
		_ = s.Mirror.UpsertSessionMeta(ctx, strings.TrimSpace(req.GetSessionId()), MirrorSessionMeta{
			Command:   "torque.stack.plan",
			Requester: strings.TrimSpace(req.GetRequester()),
		}, map[string]string{
			"stack.root":    compiled.Root,
			"stack.profile": compiled.Profile,
			"stack.name":    compiled.Plan.StackName,
		})
	}
	return &apiv1.StackPlanResult{
		Json:      string(raw),
		NodeCount: int32(len(compiled.Plan.Nodes)),
		StackName: compiled.Plan.StackName,
		Profile:   compiled.Plan.Profile,
	}, nil
}

func (s *StackServer) Apply(req *apiv1.StackRunRequest, stream apiv1.StackService_ApplyServer) error {
	if req != nil {
		req.Command = "apply"
	}
	return s.run(req, stream)
}

func (s *StackServer) Delete(req *apiv1.StackRunRequest, stream apiv1.StackService_DeleteServer) error {
	if req != nil {
		req.Command = "delete"
	}
	return s.run(req, stream)
}

func (s *StackServer) Status(ctx context.Context, req *apiv1.StackStatusRequest) (*apiv1.StackStatusResult, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if req.GetFollow() {
		return nil, status.Error(codes.InvalidArgument, "stack status follow is not supported by unary Status; use Apply/Delete streaming events or MirrorService.Subscribe")
	}
	root, err := stackRootFromPath(req.GetConfigPath(), req.GetRoot())
	if err != nil {
		return nil, stackStatusError("resolve stack root", err)
	}
	format := strings.ToLower(strings.TrimSpace(req.GetFormat()))
	if format == "" {
		format = "json"
	}
	switch format {
	case "json", "raw":
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported stack status format %q (expected json|raw)", format)
	}
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		runID, err = stack.LoadMostRecentRun(root)
		if err != nil {
			return nil, stackStatusError("load most recent stack run", err)
		}
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	var out bytes.Buffer
	if err := stack.RunStatus(ctx, stack.StatusOptions{
		RootDir: root,
		RunID:   runID,
		Limit:   limit,
		Format:  format,
	}, &out); err != nil {
		return nil, stackStatusError("read stack status", err)
	}
	return &apiv1.StackStatusResult{Json: out.String(), RunId: runID}, nil
}

func (s *StackServer) run(req *apiv1.StackRunRequest, stream interface {
	Send(*apiv1.StackEvent) error
	Context() context.Context
}) (retErr error) {
	if req == nil {
		return status.Error(codes.InvalidArgument, "request is required")
	}
	cmd := strings.ToLower(strings.TrimSpace(req.GetCommand()))
	if cmd != "apply" && cmd != "delete" {
		return status.Errorf(codes.InvalidArgument, "stack command must be apply|delete (got %q)", req.GetCommand())
	}
	ctx := stream.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	compiled, err := compileStackForAgent(ctx, req.GetSelector())
	if err != nil {
		return stackStatusError("compile stack", err)
	}
	opts, err := stackRunOptionsForAgent(ctx, cmd, compiled, req.GetOptions())
	if err != nil {
		return stackStatusError("build stack run options", err)
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	producer := "stack"
	if strings.TrimSpace(req.GetRequester()) != "" {
		producer = "stack:" + strings.TrimSpace(req.GetRequester())
	}
	exitCode := int32(0)
	if s.Mirror != nil && sessionID != "" {
		_ = s.Mirror.UpsertSessionMeta(ctx, sessionID, MirrorSessionMeta{
			Command:     "torque.stack." + cmd,
			Requester:   strings.TrimSpace(req.GetRequester()),
			KubeContext: compiled.KubeContext,
		}, map[string]string{
			"stack.root":    compiled.Root,
			"stack.profile": compiled.Profile,
			"stack.name":    compiled.Plan.StackName,
		})
		_ = s.Mirror.UpsertSessionStatus(ctx, sessionID, MirrorSessionStatus{State: MirrorSessionStateRunning})
		defer func() {
			st := MirrorSessionStatus{
				State:             MirrorSessionStateDone,
				ExitCode:          exitCode,
				CompletedUnixNano: time.Now().UTC().UnixNano(),
			}
			if retErr != nil {
				if errors.Is(retErr, context.Canceled) {
					st.ExitCode = 130
					st.ErrorMessage = "canceled"
				} else {
					st.State = MirrorSessionStateError
					st.ExitCode = 1
					st.ErrorMessage = retErr.Error()
				}
			}
			_ = s.Mirror.UpsertSessionStatus(context.Background(), sessionID, st)
		}()
	}

	var sendMu sync.Mutex
	var sendErr error
	opts.EventObservers = append(opts.EventObservers, stack.RunEventObserverFunc(func(ev stack.RunEvent) {
		raw, err := json.Marshal(ev)
		if err != nil {
			return
		}
		pb := &apiv1.StackEvent{
			TimestampUnixNano: stackEventUnixNano(ev),
			Json:              string(raw),
			Terminal:          ev.Type == string(stack.RunCompleted) || ev.Type == string(stack.RunFinalized),
			RunId:             strings.TrimSpace(ev.RunID),
			Type:              strings.TrimSpace(ev.Type),
		}
		sendMu.Lock()
		defer sendMu.Unlock()
		if sendErr != nil {
			return
		}
		if err := stream.Send(pb); err != nil {
			sendErr = err
			return
		}
		if s.Mirror != nil && sessionID != "" {
			_, _, _ = s.Mirror.ingestFrame(context.Background(), &apiv1.MirrorFrame{
				SessionId: sessionID,
				Producer:  producer,
				Payload:   &apiv1.MirrorFrame_Stack{Stack: pb},
			})
		}
	}))

	var stdout, stderr bytes.Buffer
	err = stack.Run(ctx, opts, &stdout, &stderr)
	if err != nil {
		retErr = err
		return stackStatusError("run stack "+cmd, err)
	}
	sendMu.Lock()
	defer sendMu.Unlock()
	if sendErr != nil {
		retErr = sendErr
		return sendErr
	}
	return nil
}

func compileStackForAgent(ctx context.Context, sel *apiv1.StackSelector) (stackCompileResult, error) {
	if sel == nil {
		sel = &apiv1.StackSelector{}
	}
	root, err := stackRootFromPath(sel.GetConfigPath(), sel.GetRoot())
	if err != nil {
		return stackCompileResult{}, err
	}
	u, err := stack.Discover(root)
	if err != nil {
		return stackCompileResult{}, err
	}
	profile := strings.TrimSpace(sel.GetProfile())
	if profile == "" {
		profile = strings.TrimSpace(u.DefaultProfile)
	}
	cli, err := stack.ResolveStackCLIConfig(u, profile)
	if err != nil {
		return stackCompileResult{}, err
	}
	clusters := append([]string(nil), cli.Clusters...)
	if len(sel.GetClusters()) > 0 {
		clusters = splitStackCSV(sel.GetClusters())
	}
	selector := cli.Selector
	if len(sel.GetTags()) > 0 {
		selector.Tags = splitStackCSV(sel.GetTags())
	}
	if len(sel.GetFromPaths()) > 0 {
		selector.FromPaths = splitStackCSV(sel.GetFromPaths())
	}
	if len(sel.GetReleases()) > 0 {
		selector.Releases = splitStackCSV(sel.GetReleases())
	}
	if strings.TrimSpace(sel.GetGitRange()) != "" {
		selector.GitRange = strings.TrimSpace(sel.GetGitRange())
	}
	if sel.GitIncludeDeps != nil {
		selector.GitIncludeDeps = sel.GetGitIncludeDeps()
	}
	if sel.GitIncludeDependents != nil {
		selector.GitIncludeDependents = sel.GetGitIncludeDependents()
	}
	if sel.IncludeDeps != nil {
		selector.IncludeDeps = sel.GetIncludeDeps()
	}
	if sel.IncludeDependents != nil {
		selector.IncludeDependents = sel.GetIncludeDependents()
	}
	if sel.AllowMissingDeps != nil {
		selector.AllowMissingDeps = sel.GetAllowMissingDeps()
	}
	inferDeps := cli.InferDeps
	if sel.InferDeps != nil {
		inferDeps = sel.GetInferDeps()
	}
	inferConfigRefs := cli.InferConfigRefs
	if sel.InferConfigRefs != nil {
		inferConfigRefs = sel.GetInferConfigRefs()
	}
	if strings.TrimSpace(selector.GitRange) == "" && (selector.GitIncludeDeps || selector.GitIncludeDependents) {
		return stackCompileResult{}, fmt.Errorf("invalid selector: git include options require git_range")
	}

	p, err := stack.Compile(u, stack.CompileOptions{Profile: profile})
	if err != nil {
		return stackCompileResult{}, err
	}
	secretProvider := strings.TrimSpace(sel.GetSecretProvider())
	secretConfig := strings.TrimSpace(sel.GetSecretConfig())
	kubeContext := strings.TrimSpace(sel.GetKubeContext())
	kubeconfigPath := strings.TrimSpace(sel.GetKubeconfigPath())
	if inferDeps {
		secretOptions, err := stackSecretOptionsForAgent(ctx, p.StackRoot, secretProvider, secretConfig)
		if err != nil {
			return stackCompileResult{}, err
		}
		if secretOptions == nil {
			secretOptions = &deploy.SecretOptions{}
		}
		if err := stack.InferDependencies(ctx, p, kubeconfigPath, kubeContext, stack.InferDepsOptions{
			IncludeConfigRefs: inferConfigRefs,
			Secrets:           secretOptions,
		}); err != nil {
			return stackCompileResult{}, err
		}
		if err := stack.RecomputeExecutionGroups(p); err != nil {
			return stackCompileResult{}, err
		}
	}
	selected, err := stack.Select(u, p, clusters, selector)
	if err != nil {
		return stackCompileResult{}, err
	}
	if selected != nil && len(selected.Nodes) == 0 {
		return stackCompileResult{}, fmt.Errorf("selection matched 0 releases")
	}
	return stackCompileResult{
		Plan:           selected,
		Root:           root,
		Profile:        profile,
		Selector:       selector,
		Clusters:       clusters,
		SecretProvider: secretProvider,
		SecretConfig:   secretConfig,
		KubeContext:    kubeContext,
		KubeconfigPath: kubeconfigPath,
		ApplyDefaults: stackRunDefaults{
			DryRun:    cli.ApplyDryRun,
			Diff:      cli.ApplyDiff,
			FailFast:  cli.ApplyFailFast,
			Retry:     cli.ApplyRetry,
			Lock:      cli.ApplyLock,
			Takeover:  cli.ApplyTakeover,
			LockTTL:   cli.ApplyLockTTL,
			LockOwner: cli.ApplyLockOwner,
		},
		DeleteDefaults: stackRunDefaults{
			FailFast:    cli.DeleteFailFast,
			Retry:       cli.DeleteRetry,
			Lock:        cli.DeleteLock,
			Takeover:    cli.DeleteTakeover,
			LockTTL:     cli.DeleteLockTTL,
			LockOwner:   cli.DeleteLockOwner,
			ConfirmSize: cli.DeleteConfirmThreshold,
		},
	}, nil
}

func stackRunOptionsForAgent(ctx context.Context, cmd string, compiled stackCompileResult, pb *apiv1.StackRunOptions) (stack.RunOptions, error) {
	if pb == nil {
		pb = &apiv1.StackRunOptions{}
	}
	defaults := compiled.ApplyDefaults
	if cmd == "delete" {
		defaults = compiled.DeleteDefaults
	}
	concurrency := compiled.Plan.Runner.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if pb.GetConcurrency() > 0 {
		concurrency = int(pb.GetConcurrency())
	}
	progressive := compiled.Plan.Runner.ProgressiveConcurrency
	if pb.ProgressiveConcurrency != nil {
		progressive = pb.GetProgressiveConcurrency()
	}
	failFast := true
	if defaults.FailFast != nil {
		failFast = *defaults.FailFast
	}
	if pb.FailFast != nil {
		failFast = pb.GetFailFast()
	}
	if pb.ContinueOnError != nil && pb.GetContinueOnError() {
		failFast = false
	}
	dryRun := false
	if defaults.DryRun != nil {
		dryRun = *defaults.DryRun
	}
	if pb.DryRun != nil {
		dryRun = pb.GetDryRun()
	}
	diff := false
	if defaults.Diff != nil {
		diff = *defaults.Diff
	}
	if pb.Diff != nil {
		diff = pb.GetDiff()
	}
	lock := true
	if defaults.Lock != nil {
		lock = *defaults.Lock
	}
	if pb.Lock != nil {
		lock = pb.GetLock()
	}
	takeover := false
	if defaults.Takeover != nil {
		takeover = *defaults.Takeover
	}
	if pb.TakeoverLock != nil {
		takeover = pb.GetTakeoverLock()
	}
	lockTTL := 30 * time.Minute
	if defaults.LockTTL != nil {
		lockTTL = *defaults.LockTTL
	}
	if pb.GetLockTtlSeconds() > 0 {
		lockTTL = time.Duration(pb.GetLockTtlSeconds()) * time.Second
	}
	lockOwner := ""
	if defaults.LockOwner != nil {
		lockOwner = *defaults.LockOwner
	}
	if strings.TrimSpace(pb.GetLockOwner()) != "" {
		lockOwner = strings.TrimSpace(pb.GetLockOwner())
	}
	maxAttempts := 1
	if defaults.Retry != nil {
		maxAttempts = maxInt(1, *defaults.Retry)
	}
	if pb.GetRetry() > 0 {
		maxAttempts = int(pb.GetRetry())
	}
	helmLogs := false
	if pb.HelmLogs != nil {
		helmLogs = pb.GetHelmLogs()
	}
	maxKind := map[string]int{}
	for k, v := range compiled.Plan.Runner.Limits.MaxParallelKind {
		maxKind[k] = v
	}
	for k, v := range pb.GetMaxParallelKind() {
		if strings.TrimSpace(k) != "" && v > 0 {
			maxKind[strings.TrimSpace(k)] = int(v)
		}
	}
	maxPerNS := compiled.Plan.Runner.Limits.MaxParallelPerNamespace
	if pb.GetMaxParallelPerNamespace() > 0 {
		maxPerNS = int(pb.GetMaxParallelPerNamespace())
	}
	groupLimit := compiled.Plan.Runner.Limits.ParallelismGroupLimit
	if pb.GetParallelismGroupLimit() > 0 {
		groupLimit = int(pb.GetParallelismGroupLimit())
	}
	adaptive := &stack.AdaptiveConcurrencyOptions{
		Min:                compiled.Plan.Runner.Adaptive.Min,
		WindowSize:         compiled.Plan.Runner.Adaptive.Window,
		RampAfterSuccesses: compiled.Plan.Runner.Adaptive.RampAfterSuccesses,
		RampMaxFailureRate: compiled.Plan.Runner.Adaptive.RampMaxFailureRate,
		CooldownSuccessesByClass: map[string]int{
			"RATE_LIMIT":  compiled.Plan.Runner.Adaptive.CooldownSevere,
			"SERVER_5XX":  compiled.Plan.Runner.Adaptive.CooldownSevere,
			"UNAVAILABLE": compiled.Plan.Runner.Adaptive.CooldownSevere,
			"TIMEOUT":     3,
			"TRANSPORT":   3,
			"CONFLICT":    1,
			"OTHER":       1,
		},
	}
	if pb.GetAdaptiveMin() > 0 {
		adaptive.Min = int(pb.GetAdaptiveMin())
	}
	if pb.GetAdaptiveWindow() > 0 {
		adaptive.WindowSize = int(pb.GetAdaptiveWindow())
	}
	if pb.GetAdaptiveRampSuccesses() > 0 {
		adaptive.RampAfterSuccesses = int(pb.GetAdaptiveRampSuccesses())
	}
	if pb.GetAdaptiveRampMaxFailureRate() > 0 {
		adaptive.RampMaxFailureRate = pb.GetAdaptiveRampMaxFailureRate()
	}
	if pb.GetAdaptiveCooldownSevere() > 0 {
		for k := range adaptive.CooldownSuccessesByClass {
			adaptive.CooldownSuccessesByClass[k] = int(pb.GetAdaptiveCooldownSevere())
		}
	}
	kubeconfig := compiled.KubeconfigPath
	kubeContext := compiled.KubeContext
	var secretOptions *deploy.SecretOptions
	var err error
	if cmd == "apply" {
		secretOptions, err = stackSecretOptionsForAgent(ctx, compiled.Plan.StackRoot, compiled.SecretProvider, compiled.SecretConfig)
		if err != nil {
			return stack.RunOptions{}, err
		}
	}
	return stack.RunOptions{
		Command:                    cmd,
		Plan:                       compiled.Plan,
		Concurrency:                concurrency,
		ProgressiveConcurrency:     progressive,
		FailFast:                   failFast,
		AutoApprove:                pb.GetYes(),
		DryRun:                     cmd == "apply" && dryRun,
		Diff:                       cmd == "apply" && diff,
		CacheApply:                 cmd == "apply" && pb.GetCacheApply(),
		Secrets:                    secretOptions,
		HelmLogs:                   helmLogs,
		KubeQPS:                    firstNonZeroFloat32(pb.GetKubeQps(), compiled.Plan.Runner.KubeQPS),
		KubeBurst:                  firstNonZeroInt(int(pb.GetKubeBurst()), compiled.Plan.Runner.KubeBurst),
		MaxConcurrencyPerNamespace: maxPerNS,
		MaxConcurrencyByKind:       maxKind,
		ParallelismGroupLimit:      groupLimit,
		Adaptive:                   adaptive,
		Lock:                       lock,
		LockOwner:                  lockOwner,
		LockTTL:                    lockTTL,
		TakeoverLock:               takeover,
		Kubeconfig:                 &kubeconfig,
		KubeContext:                &kubeContext,
		RunID:                      strings.TrimSpace(pb.GetRunId()),
		FailMode:                   firstNonEmptyString(strings.TrimSpace(pb.GetFailMode()), chooseStackFailMode(failFast)),
		MaxAttempts:                maxAttempts,
		Selector: stack.RunSelector{
			Clusters:             compiled.Clusters,
			Tags:                 compiled.Selector.Tags,
			FromPaths:            compiled.Selector.FromPaths,
			Releases:             compiled.Selector.Releases,
			GitRange:             strings.TrimSpace(compiled.Selector.GitRange),
			GitIncludeDeps:       compiled.Selector.GitIncludeDeps,
			GitIncludeDependents: compiled.Selector.GitIncludeDependents,
			IncludeDeps:          compiled.Selector.IncludeDeps,
			IncludeDependents:    compiled.Selector.IncludeDependents,
			AllowMissingDeps:     compiled.Selector.AllowMissingDeps,
		},
	}, nil
}

func stackSecretOptionsForAgent(ctx context.Context, root string, provider string, configPath string) (*deploy.SecretOptions, error) {
	cfg, baseDir, err := secretstore.LoadConfigFromApp(ctx, strings.TrimSpace(root), strings.TrimSpace(configPath))
	if err != nil {
		return nil, err
	}
	resolver, err := secretstore.NewResolver(cfg, secretstore.ResolverOptions{
		DefaultProvider: strings.TrimSpace(provider),
		Mode:            secretstore.ResolveModeValue,
		BaseDir:         baseDir,
	})
	if err != nil {
		return nil, err
	}
	return &deploy.SecretOptions{Resolver: resolver}, nil
}

func stackRootFromPath(configPath string, root string) (string, error) {
	target := strings.TrimSpace(configPath)
	if target == "" {
		target = strings.TrimSpace(root)
	}
	if target == "" {
		target = "."
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return target, nil
	}
	base := strings.ToLower(filepath.Base(target))
	switch base {
	case "stack.yaml", "stack.yml", "release.yaml", "release.yml":
		return filepath.Dir(target), nil
	default:
		return "", fmt.Errorf("stack config must point to a stack root directory or stack.yaml/release.yaml (got %q)", target)
	}
}

func stackEventUnixNano(ev stack.RunEvent) int64 {
	ts := strings.TrimSpace(ev.TS)
	if ts != "" {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			return t.UnixNano()
		}
	}
	return time.Now().UTC().UnixNano()
}

func splitStackCSV(vals []string) []string {
	var out []string
	for _, v := range vals {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func chooseStackFailMode(failFast bool) string {
	if failFast {
		return "fail-fast"
	}
	return "continue"
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstNonZeroInt(vals ...int) int {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}

func firstNonZeroFloat32(vals ...float32) float32 {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func stackStatusError(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return status.Errorf(codes.Internal, "%s: %v", prefix, err)
}
