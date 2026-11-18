package validate_test

import (
	"bytes"
	"encoding/json"
	"testing"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestNonIdempotentToolRequiresDeduplicationOrExplicitNoRetry(t *testing.T) {
	raw, err := agentarch.Spec.ReadFile("spec/schemas/tool.spec.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("tool.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("tool.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		key        string
		retries    *int
		acceptable bool
	}{
		{name: "unspecified retry behavior"},
		{name: "unsafe retry", retries: intPtr(1)},
		{name: "explicit no retry", retries: intPtr(0), acceptable: true},
		{name: "deduplicated retry", key: "operation_id", retries: intPtr(3), acceptable: true},
		{name: "deduplication without retry limit", key: "operation_id", acceptable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idempotency := map[string]any{"idempotent": false}
			if tc.key != "" {
				idempotency["key_field"] = tc.key
			}
			limits := map[string]any{"timeout_ms": 1000}
			if tc.retries != nil {
				limits["max_retries"] = *tc.retries
			}
			tool := map[string]any{"id": "write_test", "description_for_model": "Write a bounded local update.", "input_schema": map[string]any{"type": "object"}, "effect": "write", "owner": "Example Owner", "idempotency": idempotency, "limits": limits, "permissions": map[string]any{"network": map[string]any{"deny_by_default": true, "egress": []any{}}, "sandbox": "subprocess"}}
			data, _ := json.Marshal(map[string]any{"schema_version": "1.0", "tool": tool})
			v, _ := jsonschema.UnmarshalJSON(bytes.NewReader(data))
			err := schema.Validate(v)
			if (err == nil) != tc.acceptable {
				t.Fatalf("acceptable=%v, validation=%v", tc.acceptable, err)
			}
		})
	}
}

func intPtr(v int) *int { return &v }
