package policy

import (
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// EvaluationMatchesAgent ties an asserted result to the current subject and datasets.
// It verifies local evidence, not the authenticity of a remote model execution.
func EvaluationMatchesAgent(agent, plan, result map[string]any, planDir string) bool {
	if result["agent_id"] != agent["id"] || plan["agent_id"] != agent["id"] {
		return false
	}
	subject, _ := result["subject"].(map[string]any)
	model, _ := agent["model"].(map[string]any)
	prompts, _ := agent["prompts"].(map[string]any)
	system, _ := prompts["system"].(map[string]any)
	for key, expected := range map[string]any{"model_id": model["id"], "prompt_version": system["version"], "prompt_sha256": system["sha256"]} {
		if expected == nil || expected == "" || subject[key] != expected {
			return false
		}
	}
	context, _ := agent["context"].(map[string]any)
	rag, _ := context["rag"].(map[string]any)
	if rag["enabled"] == true && (rag["corpus_version"] == nil || subject["corpus_version"] != rag["corpus_version"]) {
		return false
	}
	measured := map[string]map[string]any{}
	list, _ := result["datasets"].([]any)
	for _, raw := range list {
		d, _ := raw.(map[string]any)
		id, _ := d["id"].(string)
		if id == "" || measured[id] != nil {
			return false
		}
		measured[id] = d
	}
	planned, _ := plan["datasets"].([]any)
	if len(planned) == 0 || len(planned) != len(measured) {
		return false
	}
	seen := map[string]bool{}
	for _, raw := range planned {
		d, _ := raw.(map[string]any)
		id, _ := d["id"].(string)
		path, _ := d["path"].(string)
		if id == "" || seen[id] || path == "" {
			return false
		}
		seen[id] = true
		actual, err := os.ReadFile(filepath.Join(planDir, path))
		if err != nil {
			return false
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(actual))
		m := measured[id]
		cases, ok := number(m["cases"])
		want, wok := number(d["cases"])
		if d["sha256"] != hash || m["sha256"] != hash || !ok || !wok || cases <= 0 || cases != want {
			return false
		}
	}
	return true
}

// EvaluationGate checks each blocking metric against the authoritative plan. A result
// cannot lower its own threshold, omit a failed metric, or pass with no observations.
func EvaluationGate(plan, result map[string]any) bool {
	if result["status"] != "measured" {
		return false
	}
	gate, _ := plan["gate"].(map[string]any)
	blocking, _ := gate["block_on"].([]any)
	if len(blocking) == 0 {
		return false
	}
	planned := metricIndex(plan["metrics"])
	measured := metricIndex(result["metrics"])
	deterministic := false
	for _, entry := range blocking {
		name, ok := entry.(string)
		if !ok {
			return false
		}
		p, pok := planned[name]
		m, mok := measured[name]
		if !pok || !mok || m["kind"] != p["kind"] {
			return false
		}
		value, vok := number(m["value"])
		threshold, tok := number(p["threshold"])
		cases, cok := number(m["cases"])
		if !vok || !tok || !cok || cases <= 0 || cases != math.Trunc(cases) {
			return false
		}
		// An LLM opinion cannot be the only failing release check. Preserve its
		// result for review; deterministic thresholds below remain authoritative.
		if p["kind"] == "llm_judge" {
			continue
		}
		if (p["kind"] != "deterministic" && p["kind"] != "human") || m["passed"] != true {
			return false
		}
		switch p["direction"] {
		case "min":
			if value < threshold {
				return false
			}
		case "max":
			if value > threshold {
				return false
			}
		default:
			return false
		}
		if p["kind"] == "deterministic" {
			deterministic = true
		}
	}
	return deterministic
}

func metricIndex(raw any) map[string]map[string]any {
	list, _ := raw.([]any)
	out := map[string]map[string]any{}
	for _, item := range list {
		m, _ := item.(map[string]any)
		name, _ := m["name"].(string)
		if name == "" {
			return nil
		}
		if _, dup := out[name]; dup {
			return nil
		}
		out[name] = m
	}
	return out
}

func number(v any) (float64, bool) {
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case int:
		n = float64(x)
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}
