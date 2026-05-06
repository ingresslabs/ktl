package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ingresslabs/torque/internal/workflows/buildsvc"
)

type cacheAdvisorArgs struct {
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
	ChangedPaths     []string `json:"changedPaths"`
	BaseImages       []string `json:"baseImages"`
}

func (s *Server) toolCacheInspect(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	_ = ctx
	var args cacheAdvisorArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	inspection, err := s.cacheInspection(args)
	if err != nil {
		return mapErrorResult("INVALID_ARGUMENTS", err, false), nil
	}
	return textResult("Cache inspection returned.", inspection, resourceLink("torque://cache/inspect", "cache-inspect", "Torque cache inspection", "application/json")), nil
}

func (s *Server) toolCachePlan(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	_ = ctx
	var args cacheAdvisorArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	inspection, err := s.cacheInspection(args)
	if err != nil {
		return mapErrorResult("INVALID_ARGUMENTS", err, false), nil
	}
	plan := s.cachePlan(args, inspection)
	return textResult("Cache plan returned.", map[string]any{
		"inspection": inspection,
		"plan":       plan,
	}, resourceLink("torque://cache/plan", "cache-plan", "Torque cache plan", "application/json")), nil
}

func (s *Server) toolCacheWarm(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args cacheAdvisorArgs
	if err := decodeArgs(raw, &args); err != nil {
		return textToolError("INVALID_ARGUMENTS", err.Error(), false, nil, nil), nil
	}
	inspection, err := s.cacheInspection(args)
	if err != nil {
		return mapErrorResult("INVALID_ARGUMENTS", err, false), nil
	}
	if len(cacheStringList(inspection, "cacheTo")) == 0 {
		return textToolError("INVALID_ARGUMENTS", "cache warm requires cacheTo or s3Cache so the remote build has an export target", false, nil, inspection), nil
	}
	if cacheAdvisorArgsContainSensitiveCacheInput(args) {
		return textToolError("INVALID_ARGUMENTS", "cache warm does not accept secret-like cache credentials; put credentials on the BuildKit daemon or workload identity", false, nil, inspection), nil
	}
	if err := requireConfirmed(s.cfg.EnableWrite, args.Safety.Confirm, "cache warm"); err != nil {
		return mapErrorResult("CONFIRMATION_REQUIRED", err, false), nil
	}
	if err := requireRemote(strings.TrimSpace(s.cfg.RemoteAgent)); err != nil {
		return mapErrorResult("REMOTE_AGENT_REQUIRED", err, false), nil
	}

	buildArgs := buildRunArgs{
		commonArgs:       args.commonArgs,
		ContextDir:       args.ContextDir,
		Dockerfile:       args.Dockerfile,
		Tags:             append([]string(nil), args.Tags...),
		Platforms:        append([]string(nil), args.Platforms...),
		BuildArgs:        append([]string(nil), args.BuildArgs...),
		Secrets:          append([]string(nil), args.Secrets...),
		CacheFrom:        append([]string(nil), args.CacheFrom...),
		CacheTo:          append([]string(nil), args.CacheTo...),
		S3Cache:          args.S3Cache,
		S3CacheRegion:    args.S3CacheRegion,
		S3CacheName:      args.S3CacheName,
		S3CacheMode:      args.S3CacheMode,
		S3CacheEndpoint:  args.S3CacheEndpoint,
		S3CachePathStyle: args.S3CachePathStyle,
		Push:             false,
		Load:             false,
		NoCache:          args.NoCache,
		Builder:          args.Builder,
		DockerContext:    args.DockerContext,
		CacheDir:         args.CacheDir,
		Mode:             args.Mode,
		ComposeFiles:     append([]string(nil), args.ComposeFiles...),
		ComposeProfiles:  append([]string(nil), args.ComposeProfiles...),
		ComposeServices:  append([]string(nil), args.ComposeServices...),
		ComposeProject:   args.ComposeProject,
		AuthFile:         args.AuthFile,
		SandboxConfig:    args.SandboxConfig,
		SandboxBin:       args.SandboxBin,
		SandboxBinds:     append([]string(nil), args.SandboxBinds...),
		SandboxWorkdir:   args.SandboxWorkdir,
		SandboxLogs:      args.SandboxLogs,
		LogFile:          args.LogFile,
	}
	buildRaw, err := json.Marshal(buildArgs)
	if err != nil {
		return mapErrorResult("INVALID_ARGUMENTS", err, false), nil
	}
	buildRes, err := s.toolBuildRun(ctx, buildRaw)
	if err != nil {
		return buildRes, err
	}
	if buildRes.IsError {
		return textToolError("CACHE_WARM_FAILED", "remote cache warm build failed", true, nil, map[string]any{
			"inspection": inspection,
			"build":      buildRes.StructuredContent,
		}), nil
	}
	return textResult("Remote cache warm completed.", map[string]any{
		"inspection": inspection,
		"warm":       buildRes.StructuredContent,
	}, cacheResultLinks(buildRes)...), nil
}

func (s *Server) cacheInspection(args cacheAdvisorArgs) (map[string]any, error) {
	contextDir := firstNonEmpty(args.ContextDir, ".")
	dockerfile := firstNonEmpty(args.Dockerfile, "Dockerfile")
	defaultName := buildsvc.DefaultS3CacheName(contextDir, args.Tags)
	s3 := buildsvc.S3CacheOptions{
		Ref:          args.S3Cache,
		Region:       args.S3CacheRegion,
		Name:         args.S3CacheName,
		Mode:         args.S3CacheMode,
		EndpointURL:  args.S3CacheEndpoint,
		UsePathStyle: args.S3CachePathStyle,
	}
	cacheFrom, cacheTo, err := buildsvc.ApplyS3Cache(append([]string(nil), args.CacheFrom...), append([]string(nil), args.CacheTo...), s3, defaultName)
	if err != nil {
		return nil, err
	}

	state := "cacheless"
	if args.NoCache {
		state = "disabled"
	} else if strings.TrimSpace(args.S3Cache) != "" {
		state = "s3-enabled"
	} else if len(cacheFrom) > 0 || len(cacheTo) > 0 {
		state = "configured"
	}

	warnings := cacheWarnings(args, cacheFrom, cacheTo)
	recommendations := cacheRecommendations(args, cacheFrom, cacheTo)
	out := map[string]any{
		"state":        state,
		"isolationKey": cacheIsolationKey(args, defaultName),
		"target": map[string]any{
			"contextDir":      contextDir,
			"dockerfile":      dockerfile,
			"tags":            append([]string(nil), args.Tags...),
			"platforms":       append([]string(nil), args.Platforms...),
			"mode":            firstNonEmpty(args.Mode, "auto"),
			"composeFiles":    append([]string(nil), args.ComposeFiles...),
			"composeServices": append([]string(nil), args.ComposeServices...),
			"baseImages":      append([]string(nil), args.BaseImages...),
		},
		"s3": map[string]any{
			"enabled":        strings.TrimSpace(args.S3Cache) != "",
			"ref":            s.redactCacheText(strings.TrimSpace(args.S3Cache)),
			"region":         strings.TrimSpace(args.S3CacheRegion),
			"name":           firstNonEmpty(args.S3CacheName, defaultName),
			"mode":           firstNonEmpty(strings.ToLower(strings.TrimSpace(args.S3CacheMode)), "max"),
			"endpointUrl":    s.redactCacheText(strings.TrimSpace(args.S3CacheEndpoint)),
			"pathStyle":      args.S3CachePathStyle,
			"defaultName":    defaultName,
			"credentials":    "BuildKit daemon or workload identity; not accepted through MCP arguments",
			"manifestSource": cacheManifestSource(args),
		},
		"cacheFrom":       s.safeCacheValues(cacheFrom),
		"cacheTo":         s.safeCacheValues(cacheTo),
		"imports":         s.cacheSpecSummaries(cacheFrom),
		"exports":         s.cacheSpecSummaries(cacheTo),
		"warnings":        warnings,
		"recommendations": recommendations,
		"policy": map[string]any{
			"warmRequiresWriteEnable": true,
			"warmRequiresConfirm":     true,
			"remoteAgentConfigured":   strings.TrimSpace(s.cfg.RemoteAgent) != "",
		},
	}
	if strings.TrimSpace(args.CacheDir) != "" {
		out["cacheDir"] = inspectCacheDir(args.CacheDir)
	}
	return out, nil
}

func (s *Server) cachePlan(args cacheAdvisorArgs, inspection map[string]any) map[string]any {
	cacheTo := cacheStringList(inspection, "cacheTo")
	cacheFrom := cacheStringList(inspection, "cacheFrom")
	changes := classifyChangedPaths(args)
	strategy := "none"
	switch {
	case args.NoCache:
		strategy = "disabled"
	case strings.TrimSpace(args.S3Cache) != "":
		strategy = "s3-shared"
	case len(cacheTo) > 0:
		strategy = "export-configured"
	case len(cacheFrom) > 0:
		strategy = "import-only"
	}
	warmTargets := []map[string]any{}
	if len(cacheTo) > 0 {
		warmTargets = append(warmTargets, map[string]any{
			"contextDir":       firstNonEmpty(args.ContextDir, "."),
			"dockerfile":       firstNonEmpty(args.Dockerfile, "Dockerfile"),
			"tags":             append([]string(nil), args.Tags...),
			"platforms":        append([]string(nil), args.Platforms...),
			"composeFiles":     append([]string(nil), args.ComposeFiles...),
			"composeServices":  append([]string(nil), args.ComposeServices...),
			"cacheFrom":        cacheFrom,
			"cacheTo":          cacheTo,
			"writeTool":        "torque.cache.warm",
			"requires":         []string{"torque-mcp --enable-write", "safety.confirm=true", "remote torque-agent BuildService.RunBuild"},
			"expectedEvidence": []string{"warm.sessionId", "BuildService.RunBuild result", "normalized cache exports"},
		})
	}
	return map[string]any{
		"strategy":       strategy,
		"changedPaths":   changes,
		"warmTargets":    warmTargets,
		"agentWorkflow":  cacheAgentWorkflow(strategy, len(warmTargets) > 0),
		"nextTool":       "torque.cache.warm",
		"remoteServices": []string{"BuildService.RunBuild", "MirrorService.Subscribe"},
	}
}

func (s *Server) cacheSpecSummaries(values []string) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, raw := range values {
		item := map[string]any{"raw": s.redactCacheText(raw)}
		specs, err := buildsvc.ParseCacheSpecs([]string{raw})
		if err != nil {
			item["parseError"] = err.Error()
			out = append(out, item)
			continue
		}
		if len(specs) == 0 {
			out = append(out, item)
			continue
		}
		attrs := map[string]string{}
		keys := make([]string, 0, len(specs[0].Attrs))
		for key := range specs[0].Attrs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			attrs[key] = redactCacheAttr(key, s.redactCacheText(specs[0].Attrs[key]))
		}
		item["type"] = specs[0].Type
		item["attrs"] = attrs
		out = append(out, item)
	}
	return out
}

func (s *Server) safeCacheValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, s.redactCacheText(value))
	}
	return out
}

func cacheWarnings(args cacheAdvisorArgs, cacheFrom, cacheTo []string) []string {
	var warnings []string
	if args.NoCache && len(cacheFrom) > 0 {
		warnings = append(warnings, "noCache disables cache imports even though imports are configured")
	}
	if !args.NoCache && len(cacheFrom) == 0 && len(cacheTo) == 0 {
		warnings = append(warnings, "no cache imports or exports are configured")
	}
	if strings.TrimSpace(args.S3Cache) != "" && strings.TrimSpace(args.S3CacheRegion) == "" && strings.TrimSpace(os.Getenv("AWS_REGION")) == "" && strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION")) == "" {
		warnings = append(warnings, "s3CacheRegion is empty; BuildKit will rely on daemon AWS configuration")
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(args.S3CacheEndpoint)), "http://") {
		warnings = append(warnings, "s3CacheEndpointUrl is not HTTPS")
	}
	if cacheAdvisorArgsContainSensitiveCacheInput(args) {
		warnings = append(warnings, "cache inputs include secret-like credentials; use daemon credentials or workload identity instead")
	}
	for _, arg := range args.BuildArgs {
		if looksSensitiveValue(arg) {
			warnings = append(warnings, "buildArgs include secret-looking values; use secret provider references instead")
			break
		}
	}
	for _, secret := range args.Secrets {
		if strings.Contains(secret, "=") {
			warnings = append(warnings, "secrets should be secret IDs or provider references, not KEY=VALUE literals")
			break
		}
	}
	return warnings
}

func cacheRecommendations(args cacheAdvisorArgs, cacheFrom, cacheTo []string) []string {
	var recs []string
	if strings.TrimSpace(args.S3Cache) == "" {
		recs = append(recs, "Use s3Cache for remote workers that do not share a local BuildKit cache.")
	}
	if len(cacheTo) == 0 {
		recs = append(recs, "Add cacheTo or s3Cache before calling torque.cache.warm.")
	}
	if strings.TrimSpace(args.S3Cache) != "" && strings.TrimSpace(args.S3CacheName) == "" && len(args.Tags) == 0 {
		recs = append(recs, "Set s3CacheName to keep warm manifests stable for untagged builds.")
	}
	if len(args.ChangedPaths) == 0 {
		recs = append(recs, "Pass changedPaths to torque.cache.plan for invalidation hints.")
	}
	if len(args.Platforms) > 1 {
		recs = append(recs, "Warm all requested platforms before fanout ship runs.")
	}
	return recs
}

func classifyChangedPaths(args cacheAdvisorArgs) []map[string]any {
	paths := append([]string(nil), args.ChangedPaths...)
	sort.Strings(paths)
	if len(paths) == 0 {
		return []map[string]any{{
			"class":       "unknown",
			"impact":      "No changedPaths were supplied, so the advisor can only plan from configured imports and exports.",
			"suggested":   "Call torque.cache.inspect first, then pass changedPaths from git diff to torque.cache.plan.",
			"warmBenefit": "unknown",
		}}
	}
	out := make([]map[string]any, 0, len(paths))
	dockerfile := strings.Trim(strings.ToLower(firstNonEmpty(args.Dockerfile, "Dockerfile")), "/")
	for _, path := range paths {
		clean := strings.Trim(strings.ToLower(filepath.ToSlash(filepath.Clean(path))), "/")
		class, impact, benefit := classifyChangedPath(clean, dockerfile)
		out = append(out, map[string]any{
			"path":        path,
			"class":       class,
			"impact":      impact,
			"warmBenefit": benefit,
		})
	}
	if len(args.BuildArgs) > 0 {
		out = append(out, map[string]any{
			"path":        "buildArgs",
			"class":       "build-arg",
			"impact":      "ARG-dependent layers may change even when source files are stable.",
			"warmBenefit": "high",
		})
	}
	if len(args.Secrets) > 0 {
		out = append(out, map[string]any{
			"path":        "secrets",
			"class":       "secret-mount",
			"impact":      "Secret mounts should not be copied into layers, but Dockerfile RUN steps using secrets can reduce cache reuse.",
			"warmBenefit": "medium",
		})
	}
	return out
}

func classifyChangedPath(path, dockerfile string) (class, impact, benefit string) {
	base := filepath.Base(path)
	switch {
	case path == dockerfile || base == "dockerfile" || strings.HasSuffix(path, ".dockerfile"):
		return "dockerfile", "Dockerfile edits can invalidate the changed instruction and all downstream layers.", "high"
	case isDependencyFile(base):
		return "dependency-layer", "Dependency manifest or lockfile changed; warm dependency layers before fanout builds.", "high"
	case strings.HasPrefix(path, "chart/") || strings.HasPrefix(path, "charts/") || strings.HasPrefix(path, "values/") || strings.Contains(path, "/values"):
		return "deploy-input", "Helm inputs changed; image cache may still be reusable, but ship/apply evidence should be refreshed.", "low"
	case strings.HasPrefix(path, ".github/") || strings.HasPrefix(path, ".gitlab/"):
		return "ci-input", "CI inputs changed; cache reuse depends on builder, platform, and build args selected by the workflow.", "medium"
	default:
		return "source-layer", "Source file changed; dependency and base layers should remain reusable when Dockerfile ordering is cache-friendly.", "medium"
	}
}

func isDependencyFile(base string) bool {
	switch base {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "requirements.txt", "requirements.lock", "pyproject.toml", "poetry.lock", "cargo.toml", "cargo.lock", "pom.xml", "build.gradle", "gradle.lockfile", "gemfile", "gemfile.lock", "composer.json", "composer.lock":
		return true
	default:
		return strings.HasPrefix(base, "requirements-") && strings.HasSuffix(base, ".txt")
	}
}

func cacheAgentWorkflow(strategy string, warmable bool) []map[string]any {
	workflow := []map[string]any{
		{"step": "inspect", "tool": "torque.cache.inspect", "reason": "Read normalized imports, exports, manifest name, and local cache-intel evidence."},
		{"step": "plan", "tool": "torque.cache.plan", "reason": "Classify changed paths and decide whether warming is useful."},
	}
	if warmable && strategy != "disabled" {
		workflow = append(workflow, map[string]any{"step": "warm", "tool": "torque.cache.warm", "reason": "Run a confirmed remote build that writes cache exports before ship/apply fanout."})
	}
	return workflow
}

func inspectCacheDir(cacheDir string) map[string]any {
	out := map[string]any{"path": cacheDir}
	info, err := os.Stat(cacheDir)
	if err != nil {
		out["exists"] = false
		out["error"] = err.Error()
		return out
	}
	out["exists"] = true
	out["isDir"] = info.IsDir()
	intelDir := filepath.Join(cacheDir, "torque-cache-intel")
	entries, err := os.ReadDir(intelDir)
	if err != nil {
		out["cacheIntel"] = map[string]any{"path": intelDir, "exists": false, "error": err.Error()}
		return out
	}
	reportCount := 0
	graphCount := 0
	var latestName string
	var latestMod int64
	var latestSize int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		reportCount++
		if strings.Contains(entry.Name(), "-graph") {
			graphCount++
		}
		if stat, err := entry.Info(); err == nil && stat.ModTime().Unix() >= latestMod {
			latestMod = stat.ModTime().Unix()
			latestName = entry.Name()
			latestSize = stat.Size()
		}
	}
	out["cacheIntel"] = map[string]any{
		"path":         intelDir,
		"exists":       true,
		"reports":      reportCount,
		"graphs":       graphCount,
		"latestReport": latestName,
		"latestUnix":   latestMod,
		"latestSize":   latestSize,
	}
	return out
}

func cacheIsolationKey(args cacheAdvisorArgs, defaultName string) string {
	payload := map[string]any{
		"contextDir":      firstNonEmpty(args.ContextDir, "."),
		"dockerfile":      firstNonEmpty(args.Dockerfile, "Dockerfile"),
		"tags":            append([]string(nil), args.Tags...),
		"platforms":       append([]string(nil), args.Platforms...),
		"composeFiles":    append([]string(nil), args.ComposeFiles...),
		"composeServices": append([]string(nil), args.ComposeServices...),
		"s3Cache":         strings.TrimSpace(args.S3Cache),
		"s3CacheName":     firstNonEmpty(args.S3CacheName, defaultName),
		"s3CacheMode":     firstNonEmpty(strings.ToLower(strings.TrimSpace(args.S3CacheMode)), "max"),
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:12])
}

func cacheManifestSource(args cacheAdvisorArgs) string {
	switch {
	case strings.TrimSpace(args.S3CacheName) != "":
		return "s3CacheName"
	case len(args.Tags) > 0:
		return "first tag"
	default:
		return "context directory"
	}
}

func cacheStringList(m map[string]any, key string) []string {
	raw, ok := m[key]
	if !ok {
		return nil
	}
	switch values := raw.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if str, ok := value.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

func redactCacheAttr(key, value string) string {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if strings.Contains(upper, "TOKEN") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "KEY") || strings.Contains(upper, "AUTH") || looksSensitiveValue(value) {
		return "<redacted>"
	}
	return value
}

func cacheAdvisorArgsContainSensitiveCacheInput(args cacheAdvisorArgs) bool {
	if cacheSpecsContainSensitive(args.CacheFrom) || cacheSpecsContainSensitive(args.CacheTo) {
		return true
	}
	for _, value := range []string{args.S3Cache, args.S3CacheEndpoint} {
		if looksSensitiveValue(value) || redactURLSensitiveParts(value) != value {
			return true
		}
	}
	return false
}

func cacheSpecsContainSensitive(values []string) bool {
	for _, raw := range values {
		if looksSensitiveValue(raw) || redactURLSensitiveParts(raw) != raw {
			return true
		}
		specs, err := buildsvc.ParseCacheSpecs([]string{raw})
		if err != nil {
			continue
		}
		for _, spec := range specs {
			for key, value := range spec.Attrs {
				if redactCacheAttr(key, value) == "<redacted>" {
					return true
				}
			}
		}
	}
	return false
}

func (s *Server) redactCacheText(value string) string {
	out := redactURLSensitiveParts(s.redactText(value))
	parts := strings.Split(out, ",")
	changed := false
	for i, part := range parts {
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if redactCacheAttr(key, val) == "<redacted>" {
			parts[i] = key + "=<redacted>"
			changed = true
		}
	}
	if changed {
		return strings.Join(parts, ",")
	}
	return out
}

func redactURLSensitiveParts(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" {
		return value
	}
	changed := false
	if u.User != nil {
		u.User = url.User("<redacted>")
		changed = true
	}
	query := u.Query()
	for key := range query {
		if redactCacheAttr(key, query.Get(key)) == "<redacted>" {
			query.Set(key, "<redacted>")
			changed = true
		}
	}
	if changed {
		u.RawQuery = query.Encode()
		return u.String()
	}
	return value
}

func cacheResultLinks(res toolResult) []contentItem {
	var links []contentItem
	for _, item := range res.Content {
		if item.Type == "resource_link" {
			links = append(links, item)
		}
	}
	return links
}
