package policy

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)

func ctx(t *testing.T, jsonSrc string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonSrc), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func mustEval(t *testing.T, src string, c map[string]any) bool {
	t.Helper()
	got, err := EvalString(src, c, now)
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	return got
}

func TestLiteralsAndOperators(t *testing.T) {
	c := ctx(t, `{}`)
	cases := map[string]bool{
		`true`:                     true,
		`false`:                    false,
		`not false`:                true,
		`1 < 2`:                    true,
		`2 <= 2`:                   true,
		`3 > 4`:                    false,
		`"a" == "a"`:               true,
		`"a" != "b"`:               true,
		`"b" in ["a", "b"]`:        true,
		`"z" in ["a", "b"]`:        false,
		`"z" not in ["a", "b"]`:    true,
		`true and false`:           false,
		`true or false`:            true,
		`(1 < 2) and (3 > 2)`:      true,
		`len([1,2,3]) == 3`:        true,
		`lower("AB") == "ab"`:      true,
		`matches("v1.2", "^v\\d")`: true,
	}
	for src, want := range cases {
		if got := mustEval(t, src, c); got != want {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
}

func TestPathTraversal(t *testing.T) {
	c := ctx(t, `{"agent":{"model":{"pinned":true,"id":"claude-x"},"autonomy":{"level":"L2_act_reversible"}}}`)
	if !mustEval(t, `agent.model.pinned == true`, c) {
		t.Error("path lookup failed")
	}
	if !mustEval(t, `agent.autonomy.level in ["L2_act_reversible","L3_act_irreversible_bounded"]`, c) {
		t.Error("membership over a path failed")
	}
	// A missing path is null, and comparisons against null are false rather than an error.
	if mustEval(t, `agent.nope.deeper == true`, c) {
		t.Error("missing path should not compare equal")
	}
}

func TestMultiIsElementWise(t *testing.T) {
	c := ctx(t, `{"tools":[
	  {"tool":{"effect":"read","permissions":{"network":{"deny_by_default":true}}}},
	  {"tool":{"effect":"write","permissions":{"network":{"deny_by_default":true}}}}
	]}`)
	if !mustEval(t, `all(tools[].tool.permissions.network.deny_by_default == true)`, c) {
		t.Error("all() over a multi failed")
	}
	if !mustEval(t, `any(tools[].tool.effect == "write")`, c) {
		t.Error("any() over a multi failed")
	}
	if mustEval(t, `all(tools[].tool.effect == "read")`, c) {
		t.Error("all() must be false when one element differs")
	}
}

// An empty collection makes all() vacuously true and any() false. A control about tools has to
// hold for an agent with no tools, or every tool-related rule would block trivial agents.
func TestEmptyMultiSemantics(t *testing.T) {
	c := ctx(t, `{"tools":[]}`)
	if !mustEval(t, `all(tools[].tool.effect == "read")`, c) {
		t.Error("all() over an empty multi must be true")
	}
	if mustEval(t, `any(tools[].tool.effect == "read")`, c) {
		t.Error("any() over an empty multi must be false")
	}
	missing := ctx(t, `{}`)
	if !mustEval(t, `all(tools[].tool.effect == "read")`, missing) {
		t.Error("[] over a missing key must behave as an empty multi, not an error")
	}
}

// A multi that never passes through all() or any() is an error. "some tool denies egress" and
// "every tool denies egress" are different claims, and guessing which the author meant is how a
// control silently checks the wrong thing.
func TestUnreducedMultiIsRejected(t *testing.T) {
	c := ctx(t, `{"tools":[{"tool":{"effect":"read"}}]}`)
	_, err := EvalString(`tools[].tool.effect == "read"`, c, now)
	if err == nil {
		t.Fatal("expected an error for an unreduced multi")
	}
	if !strings.Contains(err.Error(), "all()") {
		t.Errorf("error should tell the author what to do, got: %v", err)
	}
}

func TestDatesAndAge(t *testing.T) {
	c := ctx(t, `{"evals":{"completed_at":"2026-07-01"},"agent":{"evaluation":{"max_result_age_days":30}}}`)
	if !mustEval(t, `age_days(evals.completed_at) <= agent.evaluation.max_result_age_days`, c) {
		t.Error("27 days should be within a 30 day window")
	}
	stale := ctx(t, `{"evals":{"completed_at":"2026-01-01"},"agent":{"evaluation":{"max_result_age_days":30}}}`)
	if mustEval(t, `age_days(evals.completed_at) <= agent.evaluation.max_result_age_days`, stale) {
		t.Error("a result from January must be stale in July")
	}
	// A missing date yields null, and comparisons against null are false — so a control
	// checking freshness fails, rather than passing because nothing was recorded.
	none := ctx(t, `{"evals":{},"agent":{"evaluation":{"max_result_age_days":30}}}`)
	if mustEval(t, `age_days(evals.completed_at) <= agent.evaluation.max_result_age_days`, none) {
		t.Error("a missing date must not satisfy a freshness check")
	}
}

// The whole point of the language: a pack is data. These must all fail to parse, not evaluate.
func TestNoCodeExecution(t *testing.T) {
	c := ctx(t, `{"agent":{"id":"x"}}`)
	hostile := []string{
		`exec("rm -rf /")`,
		`os.system("id")`,
		`__import__("os")`,
		`eval("1+1")`,
		`open("/etc/passwd")`,
		`agent.id.constructor("return 1")()`,
		`require("child_process")`,
		`{{7*7}}`,
		`$(whoami)`,
		"`id`",
	}
	for _, src := range hostile {
		if _, err := EvalString(src, c, now); err == nil {
			t.Errorf("expression %q was accepted; the language must reject it", src)
		}
	}
}

func TestMalformedIsErrorNotFalse(t *testing.T) {
	c := ctx(t, `{}`)
	for _, src := range []string{`(1 <`, `and`, `"unterminated`, `foo(`, `1 ==`} {
		if _, err := EvalString(src, c, now); err == nil {
			t.Errorf("malformed expression %q must error, not silently pass", src)
		}
	}
}

func TestLimitsAreEnforced(t *testing.T) {
	c := ctx(t, `{}`)
	if _, err := EvalString(strings.Repeat("a", MaxExprLen+1), c, now); err == nil {
		t.Error("over-long expression must be rejected")
	}
	deep := strings.Repeat("(", MaxDepth+5) + "true" + strings.Repeat(")", MaxDepth+5)
	if _, err := EvalString(deep, c, now); err == nil {
		t.Error("over-deep expression must be rejected")
	}
}

func TestUnknownFunctionRejected(t *testing.T) {
	if _, err := EvalString(`sqrt(4) == 2`, ctx(t, `{}`), now); err == nil {
		t.Fatal("the function set is closed; an unknown name must be rejected")
	}
}
