package verify

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const syntheticAWSAccessKey = "AKIA1234567890ABCDEF"

func TestScanRenderedSecretsFindsNonSecretSinkAndRedacts(t *testing.T) {
	secretData := base64.StdEncoding.EncodeToString([]byte(syntheticAWSAccessKey))
	objects, err := DecodeK8SYAML(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: prod
data:
  awsAccessKey: AKIA1234567890ABCDEF
---
apiVersion: v1
kind: Secret
metadata:
  name: app-secret
  namespace: prod
data:
  awsAccessKey: ` + secretData + `
`)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	report, err := ScanRenderedSecrets(objects, SecretScanOptions{
		Mode:        ModeBlock,
		FailOn:      SeverityHigh,
		Source:      "fixture",
		EvaluatedAt: time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !report.Blocked || report.Passed {
		t.Fatalf("expected blocked report: %#v", report)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("findings=%d, want 1: %#v", len(report.Findings), report.Findings)
	}
	finding := report.Findings[0]
	if finding.RuleID != "secret/value_aws_access_key" {
		t.Fatalf("rule=%q", finding.RuleID)
	}
	if finding.Observed == syntheticAWSAccessKey || strings.Contains(finding.Observed, syntheticAWSAccessKey) {
		t.Fatalf("observed leaked raw secret: %q", finding.Observed)
	}
	if finding.Confidence < 0.9 {
		t.Fatalf("confidence=%v", finding.Confidence)
	}
	if finding.Fix == nil || finding.Fix.Summary == "" {
		t.Fatalf("missing fix: %#v", finding)
	}
	var out bytes.Buffer
	if err := WriteSecretScanReport(&out, report); err != nil {
		t.Fatalf("write report: %v", err)
	}
	if strings.Contains(out.String(), syntheticAWSAccessKey) {
		t.Fatalf("report leaked raw secret:\n%s", out.String())
	}
}

func TestScanRenderedSecretsTracksSecretRefsWithoutFinding(t *testing.T) {
	objects, err := DecodeK8SYAML(`
apiVersion: v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  template:
    spec:
      containers:
        - name: api
          image: nginx:1.27
          env:
            - name: API_TOKEN
              value: secret://vault/prod/api#token
`)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	report, err := ScanRenderedSecrets(objects, SecretScanOptions{Mode: ModeBlock, FailOn: SeverityHigh})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("unexpected findings: %#v", report.Findings)
	}
	if report.Summary.SecretReferences != 1 {
		t.Fatalf("secret refs=%d, want 1", report.Summary.SecretReferences)
	}
}

func TestScanTextSecretsRedactsReport(t *testing.T) {
	report, err := ScanTextSecrets([]SecretTextInput{{
		Path:    "values/prod.yaml",
		Content: "githubToken: ghp_1234567890abcdefghijklmnopqr\n",
		Stage:   "source",
	}}, SecretScanOptions{Mode: ModeWarn, FailOn: SeverityHigh})
	if err != nil {
		t.Fatalf("scan text: %v", err)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("findings=%d", len(report.Findings))
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "ghp_1234567890abcdefghijklmnopqr") {
		t.Fatalf("text report leaked raw token: %s", raw)
	}
}
