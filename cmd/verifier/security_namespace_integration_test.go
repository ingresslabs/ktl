//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ingresslabs/torque/internal/verify"
)

const (
	securityE2EKubeconfigEnv = "TORQUE_SECURITY_E2E_KUBECONFIG"
	securityE2EConfirmEnv    = "TORQUE_SECURITY_E2E_CONFIRM"
	securityE2EContextEnv    = "TORQUE_SECURITY_E2E_CONTEXT"
	securityE2ENamespaceEnv  = "TORQUE_SECURITY_E2E_NAMESPACE"
)

func TestVerifierSecurityProfile_LiveNamespaceSecretsBoundaries(t *testing.T) {
	chdirSecurityE2ERepoRoot(t)
	kubeconfig := resolveSecurityE2EKubeconfig(t)
	if strings.TrimSpace(os.Getenv(securityE2EConfirmEnv)) != "1" {
		t.Skipf("%s=1 not set", securityE2EConfirmEnv)
	}
	ctxName := strings.TrimSpace(os.Getenv(securityE2EContextEnv))
	namespace := strings.TrimSpace(os.Getenv(securityE2ENamespaceEnv))
	providedNamespace := namespace != ""
	if namespace == "" {
		namespace = fmt.Sprintf("torque-security-e2e-%d", time.Now().UTC().UnixNano())
	}
	rawSecret := strings.Join([]string{"AKIA", "1234567890", "ABCDEF"}, "")
	tmp := t.TempDir()
	secretsReport := filepath.Join(tmp, "secrets.json")
	verifyReport := filepath.Join(tmp, "verify.json")
	evidenceDir := filepath.Join(tmp, "evidence")

	if providedNamespace {
		kubectl(t, kubeconfig, ctxName, "get", "namespace", namespace)
	} else {
		applySecurityE2ENamespace(t, kubeconfig, ctxName, namespace)
		t.Cleanup(func() {
			cleanupSecurityE2E(t, kubeconfig, ctxName, "delete", "namespace", namespace, "--ignore-not-found=true", "--wait=true", "--timeout=60s")
		})
	}
	t.Cleanup(func() {
		cleanupSecurityE2E(t, kubeconfig, ctxName, "-n", namespace, "delete", "secret/allowed-secret", "configmap/blocked-config", "pod/env-leak", "--ignore-not-found=true", "--wait=true", "--timeout=60s")
	})
	cleanupSecurityE2E(t, kubeconfig, ctxName, "-n", namespace, "delete", "secret/allowed-secret", "configmap/blocked-config", "pod/env-leak", "--ignore-not-found=true", "--wait=true", "--timeout=60s")
	applyLiveSecurityFixture(t, kubeconfig, ctxName, namespace, rawSecret)

	cmd := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	args := []string{
		"--kubeconfig", kubeconfig,
		"--namespace", namespace,
		"--security-profile", "enterprise",
		"--secrets-report", secretsReport,
		"--security-evidence", evidenceDir,
		"--format", "json",
		"--report", verifyReport,
	}
	if ctxName != "" {
		args = append([]string{"--context", ctxName}, args...)
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "verify blocked") {
		t.Fatalf("expected live namespace security scan to block, err=%v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}

	report := readSecurityE2EVerifyReport(t, verifyReport)
	secretScan := readSecurityE2ESecretsReport(t, secretsReport)
	assertSecurityE2EFindings(t, report.Findings, secretScan.Findings)
	for _, path := range []string{
		verifyReport,
		secretsReport,
		filepath.Join(evidenceDir, "manifest.json"),
		filepath.Join(evidenceDir, "secrets.report.json"),
		filepath.Join(evidenceDir, "verifier.report.json"),
		filepath.Join(evidenceDir, "redaction.proof.json"),
		filepath.Join(evidenceDir, "reports", "security.md"),
	} {
		assertSecurityE2ENoRawSecret(t, path, rawSecret)
	}
}

func chdirSecurityE2ERepoRoot(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}

func resolveSecurityE2EKubeconfig(t *testing.T) string {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(securityE2EKubeconfigEnv))
	if raw == "" {
		t.Skipf("%s not set", securityE2EKubeconfigEnv)
	}
	if strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("resolve home: %v", err)
		}
		raw = filepath.Join(home, strings.TrimPrefix(raw, "~/"))
	}
	path := filepath.Clean(raw)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat kubeconfig %s: %v", path, err)
	}
	return path
}

func applyLiveSecurityFixture(t *testing.T, kubeconfig, ctxName, namespace, rawSecret string) {
	t.Helper()
	kubectl(t, kubeconfig, ctxName, "-n", namespace, "create", "secret", "generic", "allowed-secret", "--from-literal=apiKey="+rawSecret)
	kubectl(t, kubeconfig, ctxName, "-n", namespace, "create", "configmap", "blocked-config", "--from-literal=apiKey="+rawSecret)
	kubectl(t, kubeconfig, ctxName, "-n", namespace, "run", "env-leak", "--image=registry.k8s.io/pause:3.9", "--restart=Never", "--env=API_KEY="+rawSecret)
}

func applySecurityE2ENamespace(t *testing.T, kubeconfig, ctxName, namespace string) {
	t.Helper()
	manifest := fmt.Sprintf(`
apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    torque.dev/e2e: security
`, namespace)
	cmd := exec.Command("kubectl", kubectlArgs(kubeconfig, ctxName, "apply", "-f", "-")...)
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl apply namespace: %v\n%s", err, out)
	}
}

func kubectl(t *testing.T, kubeconfig, ctxName string, args ...string) {
	t.Helper()
	cmd := exec.Command("kubectl", kubectlArgs(kubeconfig, ctxName, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func cleanupSecurityE2E(t *testing.T, kubeconfig, ctxName string, args ...string) {
	t.Helper()
	cmd := exec.Command("kubectl", kubectlArgs(kubeconfig, ctxName, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("kubectl cleanup %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func kubectlArgs(kubeconfig, ctxName string, args ...string) []string {
	out := []string{"--kubeconfig", kubeconfig}
	if strings.TrimSpace(ctxName) != "" {
		out = append(out, "--context", strings.TrimSpace(ctxName))
	}
	return append(out, args...)
}

func readSecurityE2EVerifyReport(t *testing.T, path string) *verify.Report {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read verifier report: %v", err)
	}
	var report verify.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("decode verifier report: %v\n%s", err, raw)
	}
	return &report
}

func readSecurityE2ESecretsReport(t *testing.T, path string) *verify.SecretScanReport {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read secrets report: %v", err)
	}
	var report verify.SecretScanReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("decode secrets report: %v\n%s", err, raw)
	}
	return &report
}

func assertSecurityE2EFindings(t *testing.T, verifierFindings, secretFindings []verify.Finding) {
	t.Helper()
	all := append(append([]verify.Finding{}, verifierFindings...), secretFindings...)
	var configMapLeak, envLeak, secretLeak bool
	for _, finding := range all {
		if finding.RuleID != "secret/value_aws_access_key" {
			continue
		}
		switch finding.Subject.Kind {
		case "ConfigMap":
			if finding.Subject.Name == "blocked-config" {
				configMapLeak = true
			}
		case "Pod":
			if finding.Subject.Name == "env-leak" && strings.Contains(finding.FieldPath, "env") {
				envLeak = true
			}
		case "Secret":
			secretLeak = true
		}
	}
	if !configMapLeak {
		t.Fatalf("missing ConfigMap secret-flow finding: %#v", all)
	}
	if !envLeak {
		t.Fatalf("missing env secret-flow finding: %#v", all)
	}
	if secretLeak {
		t.Fatalf("Secret object should be an allowed materialization, got finding: %#v", all)
	}
}

func assertSecurityE2ENoRawSecret(t *testing.T, path, rawSecret string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact %s: %v", path, err)
	}
	if strings.Contains(string(raw), rawSecret) {
		t.Fatalf("artifact %s leaked raw secret:\n%s", path, raw)
	}
}
