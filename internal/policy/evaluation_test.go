package policy

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluationIsBoundToCurrentSubjectAndDataset(t *testing.T) {
	for _, mutation := range []string{"none", "model", "prompt", "corpus", "dataset", "cases", "duplicate", "agent"} {
		t.Run(mutation, func(t *testing.T) {
			dir := t.TempDir()
			data := []byte("{\"id\":\"case\"}\n")
			hash := fmt.Sprintf("%x", sha256.Sum256(data))
			if err := os.WriteFile(filepath.Join(dir, "cases.jsonl"), data, 0600); err != nil {
				t.Fatal(err)
			}
			subject := map[string]any{"model_id": "model-v1", "prompt_version": "1.0.0", "prompt_sha256": "hash-v1", "corpus_version": "corpus-v1"}
			agent := map[string]any{"id": "sample", "model": map[string]any{"id": "model-v1"}, "prompts": map[string]any{"system": map[string]any{"version": "1.0.0", "sha256": "hash-v1"}}, "context": map[string]any{"rag": map[string]any{"enabled": true, "corpus_version": "corpus-v1"}}}
			plan := map[string]any{"agent_id": "sample", "datasets": []any{map[string]any{"id": "test", "path": "cases.jsonl", "sha256": hash, "cases": 1.0}}}
			dataset := map[string]any{"id": "test", "sha256": hash, "cases": 1.0}
			result := map[string]any{"agent_id": "sample", "subject": subject, "datasets": []any{dataset}}
			switch mutation {
			case "model":
				subject["model_id"] = "old"
			case "prompt":
				subject["prompt_sha256"] = "old"
			case "corpus":
				subject["corpus_version"] = "old"
			case "dataset":
				os.WriteFile(filepath.Join(dir, "cases.jsonl"), []byte("changed"), 0600)
			case "cases":
				dataset["cases"] = 0.0
			case "duplicate":
				result["datasets"] = []any{dataset, dataset}
			case "agent":
				result["agent_id"] = "other"
			}
			if got := EvaluationMatchesAgent(agent, plan, result, dir); got != (mutation == "none") {
				t.Fatalf("matched=%v", got)
			}
		})
	}
}

func TestEvaluationGateRejectsFalseSuccess(t *testing.T) {
	for _, name := range []string{"passing", "false", "missing", "zero-cases", "wrong-value", "relaxed-threshold", "duplicate", "not-run", "judge-only"} {
		t.Run(name, func(t *testing.T) {
			p := map[string]any{"name": "success", "kind": "deterministic", "threshold": 0.9, "direction": "min"}
			m := map[string]any{"name": "success", "kind": "deterministic", "value": 0.95, "passed": true, "cases": 10.0}
			plan := map[string]any{"gate": map[string]any{"block_on": []any{"success"}}, "metrics": []any{p}}
			result := map[string]any{"status": "measured", "metrics": []any{m}}
			switch name {
			case "false":
				m["passed"] = false
			case "missing":
				result["metrics"] = []any{}
			case "zero-cases":
				m["cases"] = 0.0
			case "wrong-value":
				m["value"] = 0.5
			case "relaxed-threshold":
				m["threshold"] = 0.1
				m["value"] = 0.2
			case "duplicate":
				result["metrics"] = []any{m, m}
			case "not-run":
				result["status"] = "not_run"
			case "judge-only":
				p["kind"] = "llm_judge"
				m["kind"] = "llm_judge"
			}
			if got := EvaluationGate(plan, result); got != (name == "passing") {
				t.Fatalf("gate=%v", got)
			}
		})
	}
}

func TestEvidenceLabelsAreNotProof(t *testing.T) {
	dims := Score([]Result{{ControlID: "control.ai.eval.gate_thresholds", Passed: true, Evidence: []string{"eval_result"}}})
	if len(dims) != 1 || dims[0].Proven != 0 {
		t.Fatal("unverified artifact labels received proven credit")
	}
}

func TestJudgeOpinionCannotBeTheOnlyReleaseBlocker(t *testing.T) {
	plan := map[string]any{"gate": map[string]any{"block_on": []any{"schema", "quality"}}, "metrics": []any{
		map[string]any{"name": "schema", "kind": "deterministic", "threshold": 1.0, "direction": "min"},
		map[string]any{"name": "quality", "kind": "llm_judge", "threshold": 0.9, "direction": "min"}}}
	result := map[string]any{"status": "measured", "metrics": []any{
		map[string]any{"name": "schema", "kind": "deterministic", "value": 1.0, "passed": true, "cases": 1.0},
		map[string]any{"name": "quality", "kind": "llm_judge", "value": 0.2, "passed": false, "cases": 1.0}}}
	if !EvaluationGate(plan, result) {
		t.Fatal("advisory judge alone blocked release")
	}
	result["metrics"].([]any)[0].(map[string]any)["passed"] = false
	if EvaluationGate(plan, result) {
		t.Fatal("deterministic failure did not block")
	}
}
