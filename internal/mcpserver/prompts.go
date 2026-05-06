package mcpserver

import (
	"fmt"
	"strings"
)

type promptDefinition struct {
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Arguments   []promptArgument `json:"arguments,omitempty"`
}

type promptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

func (s *Server) listPrompts() map[string]any {
	return map[string]any{
		"prompts": []promptDefinition{
			{
				Name:        "torque.release_review",
				Title:       "Torque Release Review",
				Description: "Verify and plan a release without changing the cluster.",
				Arguments: []promptArgument{
					{Name: "chart", Required: true},
					{Name: "release", Required: true},
					{Name: "namespace", Required: true},
				},
			},
			{
				Name:        "torque.safe_apply",
				Title:       "Torque Safe Apply",
				Description: "Run the Torque delivery gate and stop for explicit user confirmation before apply.",
				Arguments: []promptArgument{
					{Name: "chart", Required: true},
					{Name: "release", Required: true},
					{Name: "namespace", Required: true},
				},
			},
			{
				Name:        "torque.incident_diagnose",
				Title:       "Torque Incident Diagnose",
				Description: "Use remote logs, events, and capture evidence to diagnose an unhealthy workload.",
				Arguments: []promptArgument{
					{Name: "podQuery", Required: true},
					{Name: "namespace", Required: false},
				},
			},
			{
				Name:        "torque.evidence_summary",
				Title:       "Torque Evidence Summary",
				Description: "Summarize a Torque capture or remote MirrorService session.",
				Arguments: []promptArgument{
					{Name: "capturePath", Required: false},
					{Name: "sessionId", Required: false},
				},
			},
		},
	}
}

func (s *Server) getPrompt(name string, args map[string]any) (map[string]any, error) {
	name = strings.TrimSpace(name)
	switch name {
	case "torque.release_review":
		return promptMessages(name, fmt.Sprintf(`Review this Torque release without changing the cluster.

Use torque.verify.chart first, then torque.apply.plan. Summarize blockers, risky resources, image provenance, and resource links.

chart=%s
release=%s
namespace=%s`, argString(args, "chart"), argString(args, "release"), argString(args, "namespace"))), nil
	case "torque.safe_apply":
		return promptMessages(name, fmt.Sprintf(`Run a safe Torque apply flow.

Use torque.verify.chart and torque.apply.plan first. Do not call torque.apply.run until the user confirms the exact release, namespace, kube context, and rendered digest. When applying, pass safety.confirm=true and safety.nonInteractive=true.

chart=%s
release=%s
namespace=%s`, argString(args, "chart"), argString(args, "release"), argString(args, "namespace"))), nil
	case "torque.incident_diagnose":
		return promptMessages(name, fmt.Sprintf(`Diagnose this workload with Torque remote logs and events.

Use torque.logs.query with includeEvents=true. Summarize the top suspected cause, the exact supporting event or log line, and the next corrective action.

podQuery=%s
namespace=%s`, argString(args, "podQuery"), argString(args, "namespace"))), nil
	case "torque.evidence_summary":
		return promptMessages(name, fmt.Sprintf(`Summarize Torque evidence.

Use torque.capture.summarize for capture files and torque.session.get or torque.session.tail for remote sessions. Report the timeline, result, failed phase, and attachable resources.

capturePath=%s
sessionId=%s`, argString(args, "capturePath"), argString(args, "sessionId"))), nil
	default:
		return nil, fmt.Errorf("unknown prompt %q", name)
	}
}

func promptMessages(desc, text string) map[string]any {
	return map[string]any{
		"description": desc,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": map[string]any{
					"type": "text",
					"text": text,
				},
			},
		},
	}
}

func argString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}
