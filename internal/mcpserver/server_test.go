package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/plugin"
	"github.com/Everton-baptista/agenteARQ/internal/workflow"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestEveryPublishedToolSchemaCompiles(t *testing.T) {
	// Compile the actual in-memory value used by Call. A JSON round-trip in this
	// test would hide invalid Go slices, as happened for both write-tool schemas.
	for name, schema := range plugin.ToolSchemas {
		t.Run(name, func(t *testing.T) {
			c := jsonschema.NewCompiler()
			if err := c.AddResource("urn:agentarch:tool", schema); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Compile("urn:agentarch:tool"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCheckpointThroughMCPMutatesOnlyCurrentRevision(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git: %v %s", err, out)
	}
	s, err := workflow.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Init(workflow.InitOptions{Name: "Contract fixture", Goal: "Validate a synthetic progress checkpoint", Profile: "python-api", Author: "Test fixture operator"}); err != nil {
		t.Fatal(err)
	}
	request := []byte(`{"name":"record_checkpoint","arguments":{"phase":"draft","expected_revision":1,"note":"Synthetic checkpoint"}}`)
	v, err := Call(context.Background(), root, request)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := v.(workflow.Snapshot)
	if snapshot.State.Revision != 2 || len(snapshot.State.Checkpoints) != 1 {
		t.Fatalf("checkpoint was not recorded: %+v", snapshot.State)
	}
	if _, err := Call(context.Background(), root, request); err == nil || !strings.Contains(err.Error(), "revision") {
		t.Fatalf("stale request should fail CAS, got %v", err)
	}
}

func TestClosedToolsAndStrictSchemas(t *testing.T) {
	root := t.TempDir()
	for _, body := range []string{
		`{"name":"execute_shell","arguments":{"command":"echo unsafe"}}`,
		`{"name":"get_workflow","arguments":{"root":"/"}}`,
		`{"name":"record_checkpoint","arguments":{"phase":"pr_ready"}}`,
		`{"name":"verify_project","arguments":{"expected_revision":1,"image":"mutable:latest"}}`,
		`{"name":"get_workflow","arguments":null}`,
	} {
		if _, err := Call(context.Background(), root, []byte(body)); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	if _, err := Call(context.Background(), root, []byte(`{"name":"inspect_project","arguments":{}}`)); err != nil {
		t.Fatal(err)
	}
	if len(Tools()) != 5 {
		t.Fatal("tool surface changed without updating contracts")
	}
}

func TestStdioInitializeAndToolDiscovery(t *testing.T) {
	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-03-26\"}}\n{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\"}\n")
	var out bytes.Buffer
	if err := Serve(context.Background(), t.TempDir(), input, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("responses: %s", out.String())
	}
	for _, line := range lines {
		var v map[string]any
		if json.Unmarshal([]byte(line), &v) != nil || v["jsonrpc"] != "2.0" {
			t.Fatalf("invalid response %s", line)
		}
	}
	if !strings.Contains(lines[0], `"protocolVersion":"2025-03-26"`) {
		t.Fatal(lines[0])
	}
	if !strings.Contains(lines[1], `"verify_project"`) {
		t.Fatal(lines[1])
	}
}

func TestCancellationUnblocksIdleStdin(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, t.TempDir(), reader, io.Discard) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled server remained blocked on stdin")
	}
}
func TestCancelledToolDoesNotInspectOrMutate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Call(ctx, t.TempDir(), []byte(`{"name":"inspect_project","arguments":{}}`)); e != context.Canceled {
		t.Fatalf("cancelled call continued: %v", e)
	}
}
func TestStdioRejectsInvalidIDsAndOversizedRequest(t *testing.T) {
	var out bytes.Buffer
	if e := Serve(context.Background(), t.TempDir(), strings.NewReader(`{"jsonrpc":"2.0","id":{},"method":"ping"}`+"\n"), &out); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), `"error"`) {
		t.Fatal("object id accepted")
	}
	if e := Serve(context.Background(), t.TempDir(), strings.NewReader(strings.Repeat("x", 2<<20)), io.Discard); e == nil {
		t.Fatal("oversized input was silently accepted")
	}
}
func TestToolCallMetadataAndUnknownFields(t *testing.T) {
	if _, e := Call(context.Background(), t.TempDir(), []byte(`{"name":"inspect_project","arguments":{},"_meta":{"progressToken":"safe"}}`)); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{`{"name":"inspect_project","arguments":{},"root":"/"}`, `{"name":"inspect_project","arguments":{}} {}`} {
		if _, e := Call(context.Background(), t.TempDir(), []byte(raw)); e == nil {
			t.Fatalf("invalid params accepted: %s", raw)
		}
	}
}
