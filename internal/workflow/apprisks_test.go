package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func riskTree(t *testing.T, files map[string]string) *Service {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &Service{Root: root}
}

func joined(xs []string) string { return strings.Join(xs, "\n") }

func TestTablesWithoutRowLevelSecurityAreReported(t *testing.T) {
	s := riskTree(t, map[string]string{
		"supabase/migrations/001.sql": `create table public.notes (id uuid primary key, body text);
create table if not exists "profiles" (id uuid);
CREATE TABLE private.audit (id int);
alter table public.notes enable row level security;`,
	})
	got := joined(s.appRiskAdvice())
	if !strings.Contains(got, "public.profiles") {
		t.Errorf("unsecured public table not reported:\n%s", got)
	}
	if strings.Contains(got, "public.notes") {
		t.Errorf("table with RLS reported:\n%s", got)
	}
	if strings.Contains(got, "private.audit") {
		t.Errorf("a table outside the exposed schema reported:\n%s", got)
	}
}

func TestRowLevelSecurityIsOnlyCheckedForSupabaseStyleProjects(t *testing.T) {
	s := riskTree(t, map[string]string{"db/schema.sql": "create table users (id int);"})
	if got := s.appRiskAdvice(); len(got) != 0 {
		t.Fatalf("plain SQL project flagged: %v", got)
	}
	s = riskTree(t, map[string]string{
		"db/schema.sql": "create table users (id int);",
		"package.json":  `{"dependencies": {"@supabase/supabase-js": "2.0.0"}}`,
	})
	if got := joined(s.appRiskAdvice()); !strings.Contains(got, "public.users") {
		t.Fatalf("supabase dependency did not enable the RLS check:\n%s", got)
	}
}

func TestServiceRoleKeyInBrowserCodeIsReported(t *testing.T) {
	s := riskTree(t, map[string]string{
		"src/lib/db.ts":             "const k = import.meta.env.VITE_SUPABASE_SERVICE_ROLE_KEY",
		"src/components/Admin.tsx":  "// uses service_role for admin\nexport {}",
		"server/admin.ts":           "const key = process.env.SUPABASE_SERVICE_ROLE_KEY // service_role",
		"app/api/route.ts":          "export const x = 'service_role'",
		"node_modules/x/index.js":   "NEXT_PUBLIC_SERVICE_ROLE",
		"agentarch/std/example.tsx": "NEXT_PUBLIC_SERVICE_ROLE",
	})
	got := joined(s.appRiskAdvice())
	for _, want := range []string{"src/lib/db.ts", "src/components/Admin.tsx"} {
		if !strings.Contains(got, want) {
			t.Errorf("%s not reported:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"server/admin.ts", "app/api/route.ts", "node_modules", "agentarch/std"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%s reported but is server code or skipped:\n%s", unwanted, got)
		}
	}
}

func TestOpenFirebaseRulesAreReported(t *testing.T) {
	s := riskTree(t, map[string]string{
		"firestore.rules":     "service cloud.firestore { match /{d=**} { allow read, write: if true; } }",
		"storage.rules":       "service firebase.storage { match /b/{b}/o { allow read: if request.auth != null; } }",
		"database.rules.json": `{"rules": {".read": true, ".write": "auth != null"}}`,
	})
	got := joined(s.appRiskAdvice())
	if !strings.Contains(got, "firestore.rules") || !strings.Contains(got, "database.rules.json") {
		t.Errorf("open rules not reported:\n%s", got)
	}
	if strings.Contains(got, "storage.rules") {
		t.Errorf("authenticated rule reported:\n%s", got)
	}
}

func TestRiskAdviceIsAdvisoryAndPrefixed(t *testing.T) {
	s := riskTree(t, map[string]string{"firestore.rules": "allow read, write: if true;"})
	for _, a := range s.appRiskAdvice() {
		if !strings.HasPrefix(a, RiskAdvicePrefix) {
			t.Errorf("advice without the family prefix: %q", a)
		}
	}
}

func TestReleaseRecommendationsNameUnfilledSections(t *testing.T) {
	s := riskTree(t, map[string]string{ReleasePlanPath: ReleasePlanTemplate})
	if got := s.releaseRecommendations(); len(got) != 3 {
		t.Fatalf("template should leave three recommendations, got %v", got)
	}
	filled := ReleasePlanTemplatePT
	filled = strings.Replace(filled, "Recomendado. TODO: o que tem backup, onde e com que frequência, e a data do último teste real\nde restauração.", "Dump diário do banco para um bucket separado; restauração testada em 2026-11-20.", 1)
	s = riskTree(t, map[string]string{ReleasePlanPath: filled})
	got := joined(s.releaseRecommendations())
	if strings.Contains(got, "Backups and restore") || !strings.Contains(got, "Monitoring") {
		t.Fatalf("filled Portuguese section not recognised:\n%s", got)
	}
}

func TestADRWithoutSourcesGetsAdvice(t *testing.T) {
	s := riskTree(t, map[string]string{ADRDir + "/0001-architecture.md": "# ADR\n\n## Context\n\nx\n"})
	if !s.adrLacksSources() {
		t.Fatal("ADR without sources not noticed")
	}
	s = riskTree(t, map[string]string{ADRDir + "/0001-architecture.md": "# ADR\n\n## Fontes e custo\n\nPreços vistos em 2026-11-20.\n"})
	if s.adrLacksSources() {
		t.Fatal("Portuguese sources heading not recognised")
	}
}

func TestLiteralColoursOutsideTokensAreReported(t *testing.T) {
	s := riskTree(t, map[string]string{
		DesignTokensPath: "{}",
		"style.css": `:root { --color-bg: #eef1f0; --x: rgb(1 2 3); }
/* a comment with #123456
   spanning lines */
body { background: var(--color-bg); }
input { background: #ffffff; color: rgba(0,0,0,.8); }`,
		"node_modules/lib/x.css": "a { color: red; background: #000; }",
		"vendor.min.css":         "a{color:#000}",
	})
	got := joined(s.tokenColourAdvice())
	if !strings.Contains(got, "style.css:5 sets background to the literal colour #ffffff") || !strings.Contains(got, "style.css:5 sets color to the literal colour rgba") {
		t.Fatalf("literal colours not reported with their line:\n%s", got)
	}
	for _, unwanted := range []string{"--color-bg", "#123456", "node_modules", "min.css", "style.css:1"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("reported %s:\n%s", unwanted, got)
		}
	}
	if got := riskTree(t, map[string]string{"style.css": "a { color: #000 }"}).tokenColourAdvice(); len(got) != 0 {
		t.Fatalf("a product without design tokens was linted: %v", got)
	}
}

func TestUniversalBaseIsNeverGuessed(t *testing.T) {
	if _, ok := DefaultVerifierBases["universal"]; ok {
		t.Fatal("universal has a default base; it would silently pick a toolchain")
	}
	got := UniversalBaseOptions([]CheckSpec{{Program: "cargo"}, {Program: "/usr/bin/cargo"}, {Program: "./gradlew"}})
	if !strings.Contains(got, "rust:") || !strings.Contains(got, "eclipse-temurin") || strings.Count(got, "rust:") != 1 {
		t.Fatalf("options do not follow the declared programs: %s", got)
	}
	if got := UniversalBaseOptions(nil); !strings.Contains(got, "--base") {
		t.Fatalf("no guidance without checks: %s", got)
	}
}

func TestTranslucentContrastIsBlendedLikeABrowser(t *testing.T) {
	white30 := rgba{1, 1, 1, 0.3}
	black := rgba{0, 0, 0, 1}
	if r := ContrastRatio(white30, black); r > 3 || r < 2 {
		t.Fatalf("white at 30%% on black should render near #4D4D4D (about 2.5:1), got %.2f", r)
	}
}
