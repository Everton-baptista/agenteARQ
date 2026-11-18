package plugin

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/Everton-baptista/agenteARQ/internal/validate"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func testOptions() Options {
	return Options{Owner: "Ada Example", Contact: "ada@example.test", ModelProvider: "test-provider", ModelID: "test-model-2026-11-01"}
}
func writeTest(t *testing.T, root, path, body string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
func readTest(t *testing.T, root, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			m[rel] = string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestInstallCheckRemoveAllHosts(t *testing.T) {
	for _, host := range Hosts() {
		t.Run(host, func(t *testing.T) {
			root := t.TempDir()
			d, _ := Describe(host)
			initial := `{"theme":"user-choice","mcpServers":{"other":{"command":"other-tool"}}}`
			if host == "codex" {
				initial = "# user-owned config\nmodel = \"user-model\"\n[mcp_servers.other]\ncommand = \"other-tool\""
			}
			writeTest(t, root, d.ConfigPath, initial)
			opts := testOptions()
			opts.DryRun = true
			before := tree(t, root)
			preview, err := Install(root, host, opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Changes) == 0 || !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("dry run changed files or returned no plan")
			}
			opts.DryRun = false
			r, err := Install(root, host, opts)
			if err != nil {
				t.Fatal(err)
			}
			if !r.Installed || len(r.Backups) == 0 {
				t.Fatal("missing installed result or backup")
			}
			if !bytes.Equal(readTest(t, root, r.Backups[0]), []byte(initial)) {
				t.Fatal("backup is not original content")
			}
			st, err := os.Stat(filepath.Join(root, r.Backups[0]))
			if err != nil || st.Mode().Perm() != 0600 {
				t.Fatal("backup is not private")
			}
			checked, err := Check(root, host)
			if err != nil || len(checked.Issues) > 0 {
				t.Fatalf("check: %+v %v", checked, err)
			}
			after := tree(t, root)
			again, err := Install(root, host, opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(again.Changes) != 0 || !reflect.DeepEqual(after, tree(t, root)) {
				t.Fatal("installation is not idempotent")
			}
			v, err := validate.New(agentarch.Spec)
			if err != nil {
				t.Fatal(err)
			}
			findings, err := v.Project(root)
			if err != nil || len(findings) > 0 {
				t.Fatalf("invalid generated contracts: %+v %v", findings, err)
			}
			// Unrelated configuration may change after installation and must survive removal.
			cfg := readTest(t, root, d.ConfigPath)
			if host == "codex" {
				cfg = append(cfg, []byte("\n[extra]\nvalue = 7\n")...)
			} else {
				var m map[string]any
				json.Unmarshal(cfg, &m)
				m["new_user_setting"] = true
				cfg, _ = json.Marshal(m)
			}
			writeTest(t, root, d.ConfigPath, string(cfg))
			checked, err = Check(root, host)
			if err != nil || len(checked.Issues) > 0 {
				t.Fatalf("unrelated change affected check: %+v %v", checked, err)
			}
			opts.DryRun = true
			before = tree(t, root)
			if _, err = Remove(root, host, opts); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("remove dry run changed files")
			}
			opts.DryRun = false
			if _, err = Remove(root, host, opts); err != nil {
				t.Fatal(err)
			}
			remaining := readTest(t, root, d.ConfigPath)
			if strings.Contains(string(remaining), "agentarch") {
				t.Fatalf("managed config left: %s", remaining)
			}
			if host == "codex" {
				if string(remaining) != initial+"\n[extra]\nvalue = 7\n" {
					t.Fatalf("TOML not preserved: %s", remaining)
				}
			} else {
				var m map[string]any
				json.Unmarshal(remaining, &m)
				if m["theme"] != "user-choice" || m["new_user_setting"] != true || m["mcpServers"].(map[string]any)["other"] == nil {
					t.Fatal("unrelated JSON values lost")
				}
			}
			checked, err = Check(root, host)
			if err != nil || checked.Installed {
				t.Fatalf("still installed: %+v %v", checked, err)
			}
		})
	}
}

func TestManagedDriftBlocksMutations(t *testing.T) {
	for _, target := range []string{"skill", "config"} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			opts := testOptions()
			if _, err := Install(root, "claude", opts); err != nil {
				t.Fatal(err)
			}
			path := ".claude/skills/agentarch-implement-task/SKILL.md"
			if target == "config" {
				path = ".mcp.json"
				b := readTest(t, root, path)
				writeTest(t, root, path, strings.Replace(string(b), `"agentarch"`, `"different-binary"`, 1))
			} else {
				writeTest(t, root, path, "user custom instruction")
			}
			before := tree(t, root)
			report, err := Check(root, "claude")
			if err != nil || len(report.Issues) == 0 {
				t.Fatalf("drift not detected: %+v %v", report, err)
			}
			if _, err = Install(root, "claude", opts); err == nil {
				t.Fatal("install overwrote drift")
			}
			if _, err = Remove(root, "claude", opts); err == nil {
				t.Fatal("remove deleted drift")
			}
			if !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("conflict mutated project")
			}
		})
	}
}

func TestCollisionAndMalformedConfigAreNonMutating(t *testing.T) {
	for _, config := range []string{`{"mcpServers":{"agentarch":{"command":"user-tool"}}}`, `{"mcpServers":null}`, `{"mcpServers":{},"mcpServers":{}}`, `not json`} {
		t.Run(config, func(t *testing.T) {
			root := t.TempDir()
			writeTest(t, root, ".mcp.json", config)
			before := tree(t, root)
			if _, err := Install(root, "claude", testOptions()); err == nil {
				t.Fatal("expected config conflict")
			}
			if !reflect.DeepEqual(before, tree(t, root)) {
				t.Fatal("failed preflight wrote files")
			}
		})
	}
}

func TestUnownedSkillPreservedAndCollisionRefused(t *testing.T) {
	root := t.TempDir()
	p := ".claude/skills/agentarch-implement-task/SKILL.md"
	src, err := agentarch.Content.ReadFile("content/skills/agentarch-implement-task/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	writeTest(t, root, p, string(src))
	if _, err = Install(root, "claude", testOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err = Remove(root, "claude", Options{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readTest(t, root, p), src) {
		t.Fatal("preexisting file removed")
	}
	writeTest(t, root, p, "different skill owned by user")
	before := tree(t, root)
	if _, err = Install(root, "claude", testOptions()); err == nil {
		t.Fatal("expected skill collision")
	}
	if !reflect.DeepEqual(before, tree(t, root)) {
		t.Fatal("collision changed files")
	}
}

func TestRejectSymlinkAndTamperedReceipt(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".claude")); err != nil {
		t.Skip(err)
	}
	if _, err := Install(root, "claude", testOptions()); err == nil {
		t.Fatal("followed skill symlink")
	}
	if len(tree(t, outside)) != 0 {
		t.Fatal("wrote outside root")
	}
	root = t.TempDir()
	if _, err := Install(root, "claude", testOptions()); err != nil {
		t.Fatal(err)
	}
	writeTest(t, root, "precious.txt", "user data")
	s, err := readState(root, "claude")
	if err != nil {
		t.Fatal(err)
	}
	s.Entries["precious.txt"] = entry{Kind: "file", Hash: digest([]byte("user data"))}
	b, _ := json.Marshal(s)
	writeTest(t, root, stateFile("claude"), string(b))
	if _, err = Remove(root, "claude", Options{}); err == nil {
		t.Fatal("accepted out-of-namespace receipt")
	}
	if string(readTest(t, root, "precious.txt")) != "user data" {
		t.Fatal("user file lost")
	}
}

func TestInstallRequiresExplicitDeploymentFacts(t *testing.T) {
	for _, field := range []string{"owner", "contact", "provider", "model", "version"} {
		t.Run(field, func(t *testing.T) {
			opts := testOptions()
			switch field {
			case "owner":
				opts.Owner = ""
			case "contact":
				opts.Contact = ""
			case "provider":
				opts.ModelProvider = ""
			case "model":
				opts.ModelID = "latest"
			case "version":
				opts.ServerVersion = "main"
			}
			root := t.TempDir()
			if _, err := Install(root, "codex", opts); err == nil {
				t.Fatal("accepted missing or floating deployment fact")
			}
			if len(tree(t, root)) != 0 {
				t.Fatal("invalid options mutated project")
			}
		})
	}
}

func TestAllowlistPinsPublicToolContract(t *testing.T) {
	root := t.TempDir()
	if _, err := Install(root, "external", testOptions()); err != nil {
		t.Fatal(err)
	}
	b := readTest(t, root, "agentarch/project/plugins/external/mcp.allowlist.yaml")
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	schemaBytes, _ := agentarch.Spec.ReadFile("spec/schemas/mcp.allowlist.schema.json")
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		t.Fatal(err)
	}
	if err = compiler.AddResource("allowlist.json", schemaDoc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("allowlist.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(doc); err != nil {
		t.Fatal(err)
	}
	server := doc["servers"].([]any)[0].(map[string]any)
	hashes := server["tool_description_sha256"].(map[string]any)
	if len(hashes) != len(ToolDescriptions) {
		t.Fatal("incomplete description pins")
	}
	for name, desc := range ToolDescriptions {
		if hashes[name] != digest([]byte(desc)) {
			t.Fatalf("wrong hash for %s", name)
		}
	}
	if server["pin"].(map[string]any)["version"] != ServerVersion || doc["default"] != "deny" {
		t.Fatal("missing exact version or deny default")
	}
}

func TestWriteToolContractsDeclareActualWritesAndNoReplay(t *testing.T) {
	files, err := contractFiles(t.TempDir(), "external", testOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"record_checkpoint", "verify_project"} {
		var doc map[string]any
		if err := json.Unmarshal(files["agentarch/project/tools/agentarch_external_"+name+".tool.yaml"], &doc); err != nil {
			t.Fatal(err)
		}
		tool := doc["tool"].(map[string]any)
		idempotency := tool["idempotency"].(map[string]any)
		if idempotency["idempotent"] != false || idempotency["key_field"] != nil {
			t.Fatalf("%s claims deduplication", name)
		}
		if tool["limits"].(map[string]any)["max_retries"] != float64(0) {
			t.Fatalf("%s allows automatic replay", name)
		}
		writes := tool["permissions"].(map[string]any)["filesystem"].(map[string]any)["write"].([]any)
		if !reflect.DeepEqual(writes, []any{"git-dir/agentarch-workflow/**", "agentarch/project/evidence/latest.json"}) {
			t.Fatalf("%s write boundary: %v", name, writes)
		}
	}
}
