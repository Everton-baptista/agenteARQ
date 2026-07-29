package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func mapfs(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

func ids(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.ID)
	}
	return out
}

// Without this lint, "framework-neutral core" is a claim nobody checks. An example creeps into a
// standard, then a code sample, and two releases later the core is coupled to a release cycle
// it does not control.
func TestFrameworkNameInAStandardIsReported(t *testing.T) {
	got, err := LintFrameworkNeutrality(mapfs(map[string]string{
		"standards/en/03-tools.md": "Declare the tool. With LangChain you would use a Tool object.\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "AA-FWK-014" {
		t.Fatalf("expected one AA-FWK-014, got %v", ids(got))
	}
	if !strings.Contains(got[0].Fix, "adapters") {
		t.Error("the fix should point at content/adapters/")
	}
}

func TestFrameworkNameInAnAdapterIsFine(t *testing.T) {
	got, _ := LintFrameworkNeutrality(mapfs(map[string]string{
		"adapters/langgraph.md": "LangGraph state carries the verified prompt.\n",
	}))
	if len(got) != 0 {
		t.Fatalf("adapters exist to name frameworks; got %v", ids(got))
	}
}

// References map external material, so naming the thing being mapped is the point.
func TestFrameworkNameInReferencesIsFine(t *testing.T) {
	got, _ := LintFrameworkNeutrality(mapfs(map[string]string{
		"references/en/observability.md": "OpenInference is a complementary convention.\n",
	}))
	if len(got) != 0 {
		t.Fatalf("references may name what they map; got %v", ids(got))
	}
}

func TestFrameworkNameInACoreFileIsReported(t *testing.T) {
	got, _ := LintFrameworkNeutrality(mapfs(map[string]string{
		"core/en/10-invariants.md": "NEVER let CrewAI delegate without a budget.\n",
	}))
	if len(got) == 0 {
		t.Fatal("the always-loaded core is the worst place to couple to a framework")
	}
}

// A framework named inside a fenced code block is an import line, not coupling of the prose.
func TestCodeFencesAreIgnored(t *testing.T) {
	got, _ := LintFrameworkNeutrality(mapfs(map[string]string{
		"standards/en/12-observability.md": "Emit a span.\n\n```python\nfrom langgraph import x\n```\n",
	}))
	if len(got) != 0 {
		t.Fatalf("a code fence should not trip the lint; got %v", ids(got))
	}
}

// A partial word must not match: "agnostic" contains "agno".
func TestWordBoundariesAreRespected(t *testing.T) {
	got, _ := LintFrameworkNeutrality(mapfs(map[string]string{
		"standards/en/00-index.md": "The core is provider agnostic and framework agnostic.\n",
	}))
	if len(got) != 0 {
		t.Fatalf("a substring inside another word must not match; got %v", ids(got))
	}
}

func TestAdapterMissingTheActionGuardrailIsReported(t *testing.T) {
	got, err := LintAdapterCoverage(mapfs(map[string]string{
		"adapters/thing.md": "# Adapter\n\n## system prompt\n\n## permission\n\n## Telemetry\n\n## Handoff\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "AA-ADP-017" {
		t.Fatalf("an adapter skipping the guardrail section must be reported, got %v", ids(got))
	}
}

func TestCompleteAdapterIsAccepted(t *testing.T) {
	got, _ := LintAdapterCoverage(mapfs(map[string]string{
		"adapters/thing.md": "# Adapter\n\n## system prompt\n## permission\n## guardrail\n## Telemetry\n## Handoff\n",
	}))
	if len(got) != 0 {
		t.Fatalf("expected no findings, got %v", ids(got))
	}
}

func TestReadmeIsNotTreatedAsAnAdapter(t *testing.T) {
	got, _ := LintAdapterCoverage(mapfs(map[string]string{
		"adapters/README.md": "# Adapters\n\nWhat these are for.\n",
	}))
	if len(got) != 0 {
		t.Fatalf("the README is not an adapter; got %v", ids(got))
	}
}

// A skill with no checklist means the standard works better in assistants that load skills —
// which contradicts the promise it makes on its own front page.
func TestSkillWithoutChecklistIsReported(t *testing.T) {
	got, err := LintSkillChecklistParity(mapfs(map[string]string{
		"skills/agentarch-new-agent/SKILL.md": "---\nname: x\n---\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "AA-SKL-018" {
		t.Fatalf("expected AA-SKL-018, got %v", ids(got))
	}
	if !strings.Contains(got[0].Fix, "checklists/new-agent.md") {
		t.Errorf("the fix should name the file to create, got: %s", got[0].Fix)
	}
}

func TestChecklistWithoutSkillIsReported(t *testing.T) {
	got, _ := LintSkillChecklistParity(mapfs(map[string]string{
		"skills/agentarch-new-agent/SKILL.md": "---\nname: x\n---\n",
		"checklists/new-agent.md":             "# c\n",
		"checklists/orphan.md":                "# c\n",
	}))
	if len(got) != 1 {
		t.Fatalf("expected one orphan checklist, got %v", got)
	}
	if !strings.Contains(got[0].File, "orphan") {
		t.Errorf("wrong file reported: %s", got[0].File)
	}
}

func TestMatchedPairIsAccepted(t *testing.T) {
	got, _ := LintSkillChecklistParity(mapfs(map[string]string{
		"skills/agentarch-new-agent/SKILL.md": "---\nname: x\n---\n",
		"checklists/new-agent.md":             "# c\n",
		"checklists/README.md":                "# index, not a workflow\n",
	}))
	if len(got) != 0 {
		t.Fatalf("expected no findings, got %v", got)
	}
}

func TestSkillWithoutSkillMdIsReported(t *testing.T) {
	got, _ := LintSkillChecklistParity(mapfs(map[string]string{
		"skills/agentarch-new-agent/references/x.md": "x\n",
		"checklists/new-agent.md":                    "# c\n",
	}))
	if len(got) != 1 || !strings.Contains(got[0].Message, "SKILL.md") {
		t.Fatalf("a skill directory with no SKILL.md must be reported, got %v", got)
	}
}

// The shipped content must satisfy its own rule.
func TestShippedSkillsAndChecklistsAreInParity(t *testing.T) {
	d, _ := os.Getwd()
	for range 5 {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			break
		}
		d = filepath.Dir(d)
	}
	got, err := LintSkillChecklistParity(os.DirFS(filepath.Join(d, "content")))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got {
		t.Errorf("%s", f)
	}
}
