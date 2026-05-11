package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingresslabs/torque/internal/kube"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestReleasePromoteKubernetesCanaryExecuteEndToEnd(t *testing.T) {
	dir, graphPath, keyPath := writeProofGateFixture(t, true)
	outDir := filepath.Join(dir, "promote-kubernetes-canary")
	clientset := kubefake.NewSimpleClientset(
		testDeployment("api", "prod", 10, map[string]string{"app": "api", "track": "stable"}),
		testDeployment("api-canary", "prod", 0, map[string]string{"app": "api", "track": "canary"}),
	)
	withReleasePromotionKubeClient(t, &kube.Client{Clientset: clientset, Namespace: "prod"})

	report := runReleasePromoteCommandForTest(t,
		"release", "promote", graphPath,
		"--strategy", "canary",
		"--steps", "10,50,100",
		"--provider", "kubernetes",
		"--execute", "--yes",
		"--out-dir", outDir,
		"--key", keyPath,
		"--fail-below", "80",
		"--format", "json",
	)
	if !report.Passed || report.Provider != "kubernetes" || report.ProviderState == nil || !report.ProviderState.Applied {
		t.Fatalf("expected applied Kubernetes canary promotion: %#v", report)
	}
	if len(report.ProviderState.Actions) != 3 || report.ProviderState.FinalTraffic.Canary != 100 {
		t.Fatalf("expected three provider canary actions ending at 100%%: %#v", report.ProviderState)
	}
	stable, err := clientset.AppsV1().Deployments("prod").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get stable deployment: %v", err)
	}
	canary, err := clientset.AppsV1().Deployments("prod").Get(context.Background(), "api-canary", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get canary deployment: %v", err)
	}
	if deploymentReplicas(stable) != 0 || deploymentReplicas(canary) != 10 {
		t.Fatalf("expected traffic shifted by replicas, stable=%d canary=%d", deploymentReplicas(stable), deploymentReplicas(canary))
	}
	assertProviderStateArtifact(t, report, true)
}

func TestReleasePromoteKubernetesBlueGreenExecuteEndToEnd(t *testing.T) {
	dir, graphPath, keyPath := writeProofGateFixture(t, true)
	outDir := filepath.Join(dir, "promote-kubernetes-blue-green")
	smokePath := filepath.Join(dir, "smoke.json")
	if err := os.WriteFile(smokePath, []byte(`{"passed":true}`), 0o644); err != nil {
		t.Fatalf("write smoke: %v", err)
	}
	blueSelector := map[string]string{"app": "api", "color": "blue"}
	greenSelector := map[string]string{"app": "api", "color": "green"}
	clientset := kubefake.NewSimpleClientset(
		testDeployment("api-blue", "prod", 10, blueSelector),
		testDeployment("api-green", "prod", 10, greenSelector),
		testService("api", "prod", blueSelector),
		testService("api-preview", "prod", blueSelector),
	)
	withReleasePromotionKubeClient(t, &kube.Client{Clientset: clientset, Namespace: "prod"})

	report := runReleasePromoteCommandForTest(t,
		"release", "promote", graphPath,
		"--strategy", "blue-green",
		"--preview",
		"--smoke", smokePath,
		"--switch-traffic",
		"--provider", "kubernetes",
		"--execute", "--yes",
		"--out-dir", outDir,
		"--key", keyPath,
		"--fail-below", "80",
		"--format", "json",
	)
	if !report.Passed || report.ProviderState == nil || !report.ProviderState.Applied {
		t.Fatalf("expected applied Kubernetes blue/green promotion: %#v", report)
	}
	active, err := clientset.CoreV1().Services("prod").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get active service: %v", err)
	}
	preview, err := clientset.CoreV1().Services("prod").Get(context.Background(), "api-preview", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get preview service: %v", err)
	}
	if active.Spec.Selector["color"] != "green" || preview.Spec.Selector["color"] != "green" {
		t.Fatalf("expected active and preview services to point at green, active=%#v preview=%#v", active.Spec.Selector, preview.Spec.Selector)
	}
	if len(report.ProviderState.Actions) != 2 || report.ProviderState.FinalTraffic.Green != 100 {
		t.Fatalf("expected preview and switch provider actions: %#v", report.ProviderState)
	}
	assertProviderStateArtifact(t, report, true)
}

func TestReleasePromoteArgoRolloutsCanaryExecuteEndToEnd(t *testing.T) {
	dir, graphPath, keyPath := writeProofGateFixture(t, true)
	outDir := filepath.Join(dir, "promote-argo-canary")
	rollout := testRollout("api", "prod")
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), rollout)
	withReleasePromotionKubeClient(t, &kube.Client{Dynamic: dyn, Namespace: "prod"})

	report := runReleasePromoteCommandForTest(t,
		"release", "promote", graphPath,
		"--strategy", "canary",
		"--steps", "20,60,100",
		"--analysis-window", "30s",
		"--provider", "argo-rollouts",
		"--execute", "--yes",
		"--out-dir", outDir,
		"--key", keyPath,
		"--fail-below", "80",
		"--format", "json",
	)
	if !report.Passed || report.Provider != "argo-rollouts" || report.ProviderState == nil || !report.ProviderState.Applied {
		t.Fatalf("expected applied Argo Rollouts canary promotion: %#v", report)
	}
	updated, err := dyn.Resource(releasePromotionArgoRolloutGVR).Namespace("prod").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get rollout: %v", err)
	}
	steps, found, err := unstructured.NestedSlice(updated.Object, "spec", "strategy", "canary", "steps")
	if err != nil || !found {
		t.Fatalf("expected rollout canary steps: found=%v err=%v obj=%#v", found, err, updated.Object)
	}
	if len(steps) != 5 {
		t.Fatalf("expected setWeight/pause canary ladder, got %d steps: %#v", len(steps), steps)
	}
	annotations := updated.GetAnnotations()
	if annotations["torque.dev/promotion-strategy"] != "canary" {
		t.Fatalf("expected torque promotion annotation: %#v", annotations)
	}
	assertProviderStateArtifact(t, report, true)
}

func TestReleasePromoteArgoRolloutsBlueGreenExecuteEndToEnd(t *testing.T) {
	dir, graphPath, keyPath := writeProofGateFixture(t, true)
	outDir := filepath.Join(dir, "promote-argo-blue-green")
	rollout := testRollout("api", "prod")
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), rollout)
	withReleasePromotionKubeClient(t, &kube.Client{Dynamic: dyn, Namespace: "prod"})

	report := runReleasePromoteCommandForTest(t,
		"release", "promote", graphPath,
		"--strategy", "blue-green",
		"--preview",
		"--switch-traffic",
		"--provider", "argo-rollouts",
		"--execute", "--yes",
		"--out-dir", outDir,
		"--key", keyPath,
		"--fail-below", "80",
		"--format", "json",
	)
	if !report.Passed || report.Provider != "argo-rollouts" || report.ProviderState == nil || !report.ProviderState.Applied {
		t.Fatalf("expected applied Argo Rollouts blue/green promotion: %#v", report)
	}
	updated, err := dyn.Resource(releasePromotionArgoRolloutGVR).Namespace("prod").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get rollout: %v", err)
	}
	blueGreen, found, err := unstructured.NestedMap(updated.Object, "spec", "strategy", "blueGreen")
	if err != nil || !found {
		t.Fatalf("expected rollout blueGreen strategy: found=%v err=%v obj=%#v", found, err, updated.Object)
	}
	if blueGreen["activeService"] != "api" || blueGreen["previewService"] != "api-preview" || blueGreen["autoPromotionEnabled"] != true {
		t.Fatalf("unexpected blueGreen strategy: %#v", blueGreen)
	}
	assertProviderStateArtifact(t, report, true)
}

func TestReleasePromoteProviderDoesNotMutateWhenGateFails(t *testing.T) {
	dir, graphPath, keyPath := writeProofGateFixture(t, false)
	outDir := filepath.Join(dir, "promote-blocked-provider")
	clientset := kubefake.NewSimpleClientset(
		testDeployment("api", "prod", 10, map[string]string{"app": "api", "track": "stable"}),
		testDeployment("api-canary", "prod", 0, map[string]string{"app": "api", "track": "canary"}),
	)
	withReleasePromotionKubeClient(t, &kube.Client{Clientset: clientset, Namespace: "prod"})

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{
		"release", "promote", graphPath,
		"--strategy", "canary",
		"--steps", "10,100",
		"--provider", "kubernetes",
		"--execute", "--yes",
		"--out-dir", outDir,
		"--key", keyPath,
		"--fail-below", "80",
		"--format", "json",
	})
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatalf("expected provider promotion to block failed gate")
	}
	var report releasePromotionReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode blocked promotion: %v\n%s", err, out.String())
	}
	if report.Passed {
		t.Fatalf("expected blocked provider report: %#v", report)
	}
	stable, err := clientset.AppsV1().Deployments("prod").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get stable deployment: %v", err)
	}
	canary, err := clientset.AppsV1().Deployments("prod").Get(context.Background(), "api-canary", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get canary deployment: %v", err)
	}
	if deploymentReplicas(stable) != 10 || deploymentReplicas(canary) != 0 {
		t.Fatalf("blocked provider mutated deployments, stable=%d canary=%d", deploymentReplicas(stable), deploymentReplicas(canary))
	}
}

func runReleasePromoteCommandForTest(t *testing.T, args ...string) releasePromotionReport {
	t.Helper()
	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("release promote command: %v\n%s", err, out.String())
	}
	var report releasePromotionReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode promotion report: %v\n%s", err, out.String())
	}
	return report
}

func withReleasePromotionKubeClient(t *testing.T, client *kube.Client) {
	t.Helper()
	previous := newReleasePromotionKubeClient
	newReleasePromotionKubeClient = func(context.Context, string, string) (*kube.Client, error) {
		return client, nil
	}
	t.Cleanup(func() {
		newReleasePromotionKubeClient = previous
	})
}

func assertProviderStateArtifact(t *testing.T, report releasePromotionReport, applied bool) {
	t.Helper()
	if report.Artifacts.ProviderState == "" {
		t.Fatalf("expected provider state artifact: %#v", report.Artifacts)
	}
	var state releasePromotionProviderState
	raw, err := os.ReadFile(report.Artifacts.ProviderState)
	if err != nil {
		t.Fatalf("read provider state: %v", err)
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("decode provider state: %v", err)
	}
	if state.Applied != applied || len(state.Actions) == 0 {
		t.Fatalf("unexpected provider state: %#v", state)
	}
}

func testDeployment(name, namespace string, replicas int32, labels map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: copyStringMap(labels)},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: copyStringMap(labels)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: copyStringMap(labels)},
			},
		},
	}
}

func testService(name, namespace string, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       corev1.ServiceSpec{Selector: copyStringMap(selector)},
	}
}

func testRollout(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Rollout",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"strategy": map[string]any{},
		},
	}}
}
