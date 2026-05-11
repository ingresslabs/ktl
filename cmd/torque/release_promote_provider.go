package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ingresslabs/torque/internal/kube"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var releasePromotionArgoRolloutGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "rollouts",
}

var newReleasePromotionKubeClient = func(ctx context.Context, kubeconfigPath, kubeContext string) (*kube.Client, error) {
	return kube.New(ctx, kubeconfigPath, kubeContext)
}

func runReleasePromotionProvider(ctx context.Context, opts releasePromoteOptions, report releasePromotionReport, source string, state releasePromotionProviderState) (releasePromotionProviderState, error) {
	switch state.Provider {
	case "file":
		state.Applied = true
		state.Message = "file provider recorded traffic state"
		state.Actions = append(state.Actions, releasePromotionAction{
			Operation: "record-traffic-state",
			Status:    "applied",
			Message:   "deterministic provider state written after proof checks passed",
			Traffic:   state.FinalTraffic,
		})
		return state, nil
	case "kubernetes", "argo-rollouts":
		client, err := newReleasePromotionKubeClient(ctx, opts.Kubeconfig, opts.KubeContext)
		if err != nil {
			return state, fmt.Errorf("initialize %s provider: %w", state.Provider, err)
		}
		if strings.TrimSpace(state.Namespace) == "" {
			state.Namespace = strings.TrimSpace(client.Namespace)
		}
		if strings.TrimSpace(state.Namespace) == "" {
			state.Namespace = "default"
		}
		switch state.Provider {
		case "kubernetes":
			return runKubernetesReleasePromotion(ctx, client.Clientset, opts, report, state)
		case "argo-rollouts":
			return runArgoRolloutsReleasePromotion(ctx, client.Dynamic, opts, report, source, state)
		}
	}
	return state, fmt.Errorf("provider %q cannot execute traffic changes", state.Provider)
}

func runKubernetesReleasePromotion(ctx context.Context, client kubernetes.Interface, opts releasePromoteOptions, report releasePromotionReport, state releasePromotionProviderState) (releasePromotionProviderState, error) {
	if client == nil {
		return state, fmt.Errorf("kubernetes provider requires a typed Kubernetes client")
	}
	switch report.Strategy {
	case "canary":
		return runKubernetesCanaryPromotion(ctx, client, opts, report, state)
	case "blue-green":
		return runKubernetesBlueGreenPromotion(ctx, client, opts, report, state)
	default:
		return state, fmt.Errorf("unsupported Kubernetes promotion strategy %q", report.Strategy)
	}
}

func runKubernetesCanaryPromotion(ctx context.Context, client kubernetes.Interface, opts releasePromoteOptions, report releasePromotionReport, state releasePromotionProviderState) (releasePromotionProviderState, error) {
	release, err := releasePromotionReleaseName(report)
	if err != nil {
		return state, err
	}
	namespace := releasePromotionNamespace(report, state.Namespace)
	stableName := firstNonEmpty(opts.StableDeploy, release)
	canaryName := firstNonEmpty(opts.CanaryDeploy, release+"-canary")
	stable, err := client.AppsV1().Deployments(namespace).Get(ctx, stableName, metav1.GetOptions{})
	if err != nil {
		return state, fmt.Errorf("get stable deployment %s/%s: %w", namespace, stableName, err)
	}
	canary, err := client.AppsV1().Deployments(namespace).Get(ctx, canaryName, metav1.GetOptions{})
	if err != nil {
		return state, fmt.Errorf("get canary deployment %s/%s: %w", namespace, canaryName, err)
	}
	total := opts.TotalReplicas
	if total <= 0 {
		total = int(deploymentReplicas(stable) + deploymentReplicas(canary))
	}
	if total <= 0 {
		total = 1
	}
	for _, step := range report.Canary.Steps {
		canaryReplicas := int32(math.Ceil(float64(total) * float64(step.Traffic.Canary) / 100.0))
		if step.Traffic.Canary >= 100 {
			canaryReplicas = int32(total)
		}
		stableReplicas := int32(total) - canaryReplicas
		if stableReplicas < 0 {
			stableReplicas = 0
		}
		beforeCanary, afterCanary, err := updateDeploymentReplicas(ctx, client, namespace, canaryName, canaryReplicas)
		if err != nil {
			return state, err
		}
		beforeStable, afterStable, err := updateDeploymentReplicas(ctx, client, namespace, stableName, stableReplicas)
		if err != nil {
			return state, err
		}
		state.Actions = append(state.Actions, releasePromotionAction{
			Index:     step.Index,
			Operation: "scale-canary",
			Resource:  fmt.Sprintf("Deployment/%s/%s,Deployment/%s/%s", namespace, stableName, namespace, canaryName),
			Status:    "applied",
			Message:   fmt.Sprintf("stable replicas %d->%d, canary replicas %d->%d", beforeStable, afterStable, beforeCanary, afterCanary),
			Traffic:   step.Traffic,
		})
		state.Objects = append(state.Objects,
			releasePromotionObject{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Namespace:  namespace,
				Name:       stableName,
				Before:     map[string]string{"replicas": strconv.Itoa(int(beforeStable))},
				After:      map[string]string{"replicas": strconv.Itoa(int(afterStable))},
			},
			releasePromotionObject{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Namespace:  namespace,
				Name:       canaryName,
				Before:     map[string]string{"replicas": strconv.Itoa(int(beforeCanary))},
				After:      map[string]string{"replicas": strconv.Itoa(int(afterCanary))},
			},
		)
	}
	state.Applied = true
	state.Message = "native Kubernetes canary replicas applied"
	return state, nil
}

func runKubernetesBlueGreenPromotion(ctx context.Context, client kubernetes.Interface, opts releasePromoteOptions, report releasePromotionReport, state releasePromotionProviderState) (releasePromotionProviderState, error) {
	release, err := releasePromotionReleaseName(report)
	if err != nil {
		return state, err
	}
	namespace := releasePromotionNamespace(report, state.Namespace)
	blueName := firstNonEmpty(opts.BlueDeploy, release+"-blue")
	greenName := firstNonEmpty(opts.GreenDeploy, release+"-green")
	activeService := firstNonEmpty(opts.ActiveService, release)
	previewService := firstNonEmpty(opts.PreviewService, release+"-preview")
	if _, err := client.AppsV1().Deployments(namespace).Get(ctx, blueName, metav1.GetOptions{}); err != nil {
		return state, fmt.Errorf("get blue deployment %s/%s: %w", namespace, blueName, err)
	}
	green, err := client.AppsV1().Deployments(namespace).Get(ctx, greenName, metav1.GetOptions{})
	if err != nil {
		return state, fmt.Errorf("get green deployment %s/%s: %w", namespace, greenName, err)
	}
	greenSelector := selectorForDeployment(green)
	if len(greenSelector) == 0 {
		return state, fmt.Errorf("green deployment %s/%s has no selector labels", namespace, greenName)
	}
	if report.BlueGreen.Preview {
		before, after, err := updateServiceSelector(ctx, client, namespace, previewService, greenSelector)
		if err != nil {
			return state, err
		}
		state.Actions = append(state.Actions, releasePromotionAction{
			Index:     1,
			Operation: "preview-green",
			Resource:  fmt.Sprintf("Service/%s/%s", namespace, previewService),
			Status:    "applied",
			Message:   "preview service points at green deployment selector",
			Traffic:   releasePromotionTraffic{Blue: 100, Green: 0},
		})
		state.Objects = append(state.Objects, releasePromotionObject{APIVersion: "v1", Kind: "Service", Namespace: namespace, Name: previewService, Before: before, After: after})
	}
	if report.BlueGreen.SwitchTraffic {
		before, after, err := updateServiceSelector(ctx, client, namespace, activeService, greenSelector)
		if err != nil {
			return state, err
		}
		state.Actions = append(state.Actions, releasePromotionAction{
			Index:     len(state.Actions) + 1,
			Operation: "switch-traffic",
			Resource:  fmt.Sprintf("Service/%s/%s", namespace, activeService),
			Status:    "applied",
			Message:   "active service points at green deployment selector",
			Traffic:   releasePromotionTraffic{Blue: 0, Green: 100},
		})
		state.Objects = append(state.Objects, releasePromotionObject{APIVersion: "v1", Kind: "Service", Namespace: namespace, Name: activeService, Before: before, After: after})
	}
	if len(state.Actions) == 0 {
		state.Actions = append(state.Actions, releasePromotionAction{
			Operation: "observe-blue-green",
			Resource:  fmt.Sprintf("Deployment/%s/%s,Deployment/%s/%s", namespace, blueName, namespace, greenName),
			Status:    "applied",
			Message:   "blue/green provider validated blue and green deployments without switching traffic",
			Traffic:   state.FinalTraffic,
		})
	}
	state.Applied = true
	state.Message = "native Kubernetes blue/green service selectors applied"
	return state, nil
}

func runArgoRolloutsReleasePromotion(ctx context.Context, client dynamic.Interface, opts releasePromoteOptions, report releasePromotionReport, source string, state releasePromotionProviderState) (releasePromotionProviderState, error) {
	if client == nil {
		return state, fmt.Errorf("argo-rollouts provider requires a dynamic Kubernetes client")
	}
	release, err := releasePromotionReleaseName(report)
	if err != nil {
		return state, err
	}
	namespace := releasePromotionNamespace(report, state.Namespace)
	rolloutName := firstNonEmpty(opts.Rollout, release)
	resource := client.Resource(releasePromotionArgoRolloutGVR).Namespace(namespace)
	rollout, err := resource.Get(ctx, rolloutName, metav1.GetOptions{})
	if err != nil {
		return state, fmt.Errorf("get Argo Rollouts Rollout %s/%s: %w", namespace, rolloutName, err)
	}
	beforeVersion := rollout.GetResourceVersion()
	next := rollout.DeepCopy()
	annotations := next.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["torque.dev/promotion-strategy"] = report.Strategy
	annotations["torque.dev/promotion-source"] = source
	annotations["torque.dev/promotion-time"] = time.Now().UTC().Format(time.RFC3339Nano)
	next.SetAnnotations(annotations)
	switch report.Strategy {
	case "canary":
		if err := configureArgoCanary(next, report); err != nil {
			return state, err
		}
		state.Actions = append(state.Actions, releasePromotionAction{
			Operation: "configure-rollout-canary",
			Resource:  fmt.Sprintf("Rollout/%s/%s", namespace, rolloutName),
			Status:    "applied",
			Message:   "Argo Rollouts canary steps configured from proof-backed promotion plan",
			Traffic:   state.FinalTraffic,
		})
	case "blue-green":
		configureArgoBlueGreen(next, opts, report)
		state.Actions = append(state.Actions, releasePromotionAction{
			Operation: "configure-rollout-blue-green",
			Resource:  fmt.Sprintf("Rollout/%s/%s", namespace, rolloutName),
			Status:    "applied",
			Message:   "Argo Rollouts blue/green services configured from proof-backed promotion plan",
			Traffic:   state.FinalTraffic,
		})
	default:
		return state, fmt.Errorf("unsupported Argo Rollouts promotion strategy %q", report.Strategy)
	}
	updated, err := resource.Update(ctx, next, metav1.UpdateOptions{})
	if err != nil {
		return state, fmt.Errorf("update Argo Rollouts Rollout %s/%s: %w", namespace, rolloutName, err)
	}
	state.Objects = append(state.Objects, releasePromotionObject{
		APIVersion: "argoproj.io/v1alpha1",
		Kind:       "Rollout",
		Namespace:  namespace,
		Name:       rolloutName,
		Before:     map[string]string{"resourceVersion": beforeVersion},
		After:      map[string]string{"resourceVersion": updated.GetResourceVersion()},
	})
	state.Applied = true
	state.Message = "Argo Rollouts strategy applied"
	return state, nil
}

func configureArgoCanary(rollout *unstructured.Unstructured, report releasePromotionReport) error {
	if report.Canary == nil || len(report.Canary.Steps) == 0 {
		return fmt.Errorf("canary promotion requires planned steps")
	}
	steps := make([]any, 0, len(report.Canary.Steps)*2)
	for _, step := range report.Canary.Steps {
		steps = append(steps, map[string]any{"setWeight": int64(step.Traffic.Canary)})
		if step.Traffic.Canary < 100 {
			pause := map[string]any{}
			if strings.TrimSpace(report.Canary.AnalysisWindow) != "" {
				pause["duration"] = strings.TrimSpace(report.Canary.AnalysisWindow)
			}
			steps = append(steps, map[string]any{"pause": pause})
		}
	}
	unstructured.RemoveNestedField(rollout.Object, "spec", "strategy", "blueGreen")
	return unstructured.SetNestedSlice(rollout.Object, steps, "spec", "strategy", "canary", "steps")
}

func configureArgoBlueGreen(rollout *unstructured.Unstructured, opts releasePromoteOptions, report releasePromotionReport) {
	release := firstNonEmpty(report.Release, opts.Release)
	blueGreen := map[string]any{
		"activeService":        firstNonEmpty(opts.ActiveService, release),
		"previewService":       firstNonEmpty(opts.PreviewService, release+"-preview"),
		"autoPromotionEnabled": report.BlueGreen != nil && report.BlueGreen.SwitchTraffic,
	}
	unstructured.RemoveNestedField(rollout.Object, "spec", "strategy", "canary")
	_ = unstructured.SetNestedMap(rollout.Object, blueGreen, "spec", "strategy", "blueGreen")
}

func updateDeploymentReplicas(ctx context.Context, client kubernetes.Interface, namespace, name string, replicas int32) (int32, int32, error) {
	current, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return 0, 0, fmt.Errorf("get deployment %s/%s: %w", namespace, name, err)
	}
	before := deploymentReplicas(current)
	next := current.DeepCopy()
	next.Spec.Replicas = &replicas
	updated, err := client.AppsV1().Deployments(namespace).Update(ctx, next, metav1.UpdateOptions{})
	if err != nil {
		return before, before, fmt.Errorf("update deployment %s/%s replicas: %w", namespace, name, err)
	}
	return before, deploymentReplicas(updated), nil
}

func updateServiceSelector(ctx context.Context, client kubernetes.Interface, namespace, name string, selector map[string]string) (map[string]string, map[string]string, error) {
	current, err := client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("get service %s/%s: %w", namespace, name, err)
	}
	before := copyStringMap(current.Spec.Selector)
	next := current.DeepCopy()
	next.Spec.Selector = copyStringMap(selector)
	updated, err := client.CoreV1().Services(namespace).Update(ctx, next, metav1.UpdateOptions{})
	if err != nil {
		return before, before, fmt.Errorf("update service %s/%s selector: %w", namespace, name, err)
	}
	return before, copyStringMap(updated.Spec.Selector), nil
}

func deploymentReplicas(deploy *appsv1.Deployment) int32 {
	if deploy == nil || deploy.Spec.Replicas == nil {
		return 0
	}
	return *deploy.Spec.Replicas
}

func selectorForDeployment(deploy *appsv1.Deployment) map[string]string {
	if deploy == nil {
		return nil
	}
	if deploy.Spec.Selector != nil && len(deploy.Spec.Selector.MatchLabels) > 0 {
		return copyStringMap(deploy.Spec.Selector.MatchLabels)
	}
	return copyStringMap(deploy.Labels)
}

func releasePromotionReleaseName(report releasePromotionReport) (string, error) {
	release := strings.TrimSpace(report.Release)
	if release == "" {
		return "", fmt.Errorf("release name is required for traffic provider execution; pass --release")
	}
	return release, nil
}

func releasePromotionNamespace(report releasePromotionReport, fallback string) string {
	return firstNonEmpty(report.Namespace, fallback, "default")
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
