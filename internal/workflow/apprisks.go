package workflow

// Deterministic checks for the failures that keep reaching production in AI-built apps.
//
// The public incidents of the category are few and repetitive: database tables exposed through
// a public API key because row level security was never enabled (CVE-2025-48757, 170+ apps from
// one builder), a service-role key shipped inside the browser bundle, Firebase rules left at
// "allow everything". Each is visible in the source without running it, so each becomes a check
// here rather than a sentence in a prompt.
//
// They are advisory in this release. No check is born blocking (docs/03-control-authoring.md):
// they report through NextAction.Advice now and are planned to block from content 1.5, once
// their false-positive rate on real projects is known. They read files only; they never
// execute project code, and they skip dependency and build directories.

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// RiskAdvicePrefix starts every app-risk advice line, so a host or a translation can recognise
// the family without parsing the rest.
const RiskAdvicePrefix = "Security risk (advisory): "

const (
	riskMaxFiles = 20000
	riskMaxBytes = 1 << 20
)

var riskSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, ".next": true,
	".venv": true, "venv": true, "__pycache__": true, "target": true, ".turbo": true, "coverage": true,
}

var (
	createTable    = regexp.MustCompile(`(?im)^\s*create\s+table\s+(?:if\s+not\s+exists\s+)?((?:"?[a-z_][a-z0-9_]*"?\.)?"?[a-z_][a-z0-9_]*"?)`)
	enableRLS      = regexp.MustCompile(`(?im)alter\s+table\s+(?:only\s+)?(?:if\s+exists\s+)?((?:"?[a-z_][a-z0-9_]*"?\.)?"?[a-z_][a-z0-9_]*"?)\s+enable\s+row\s+level\s+security`)
	publicService  = regexp.MustCompile(`\b(?:NEXT_PUBLIC|VITE|PUBLIC|REACT_APP|EXPO_PUBLIC|NUXT_PUBLIC|GATSBY)_[A-Z0-9_]*SERVICE_ROLE[A-Z0-9_]*`)
	openFirestore  = regexp.MustCompile(`allow\s+(?:read|write|read\s*,\s*write|write\s*,\s*read|get|list|create|update|delete)[^;]*:\s*if\s+true\s*;`)
	openRealtimeDB = regexp.MustCompile(`"\.(?:read|write)"\s*:\s*(?:true|"true")`)
	clientExts     = map[string]bool{".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".vue": true, ".svelte": true, ".astro": true, ".html": true}
)

// appRiskAdvice scans the project once and returns one advice line per finding, sorted.
func (s *Service) appRiskAdvice() []string {
	var findings []string
	created := map[string]string{} // table → file that creates it
	secured := map[string]bool{}
	usesSupabase := false
	if st, err := os.Stat(filepath.Join(s.Root, "supabase")); err == nil && st.IsDir() {
		usesSupabase = true
	}
	files := 0
	_ = filepath.WalkDir(s.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(s.Root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if path != s.Root && (riskSkipDirs[d.Name()] || rel == "agentarch/std") {
				return filepath.SkipDir
			}
			return nil
		}
		if files++; files > riskMaxFiles {
			return filepath.SkipAll
		}
		if !d.Type().IsRegular() {
			return nil // symlinks, FIFOs and devices are never read
		}
		info, err := d.Info()
		if err != nil || info.Size() > riskMaxBytes {
			return nil
		}
		name := d.Name()
		ext := strings.ToLower(filepath.Ext(name))
		interesting := ext == ".sql" || clientExts[ext] || name == "package.json" || strings.HasSuffix(name, ".rules") || name == "database.rules.json"
		if !interesting {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		text := string(b)
		switch {
		case ext == ".sql":
			for _, m := range createTable.FindAllStringSubmatch(text, -1) {
				t := normalizeTable(m[1])
				if _, ok := created[t]; !ok {
					created[t] = rel
				}
			}
			for _, m := range enableRLS.FindAllStringSubmatch(text, -1) {
				secured[normalizeTable(m[1])] = true
			}
			if strings.Contains(text, "auth.uid()") || strings.Contains(text, "auth.jwt()") {
				usesSupabase = true
			}
		case name == "package.json":
			if strings.Contains(text, `"@supabase/supabase-js"`) {
				usesSupabase = true
			}
		case strings.HasSuffix(name, ".rules") || name == "database.rules.json":
			if openFirestore.MatchString(text) || openRealtimeDB.MatchString(text) {
				findings = append(findings, fmt.Sprintf("%s%s lets anyone read or write the data (a rule that is always true). Write rules that check who is signed in and what they own.", RiskAdvicePrefix, rel))
			}
		}
		if clientExts[ext] || name == "package.json" {
			if m := publicService.FindString(text); m != "" {
				findings = append(findings, fmt.Sprintf("%s%s exposes %s to the browser. A service-role key bypasses every access rule; keep it on the server only.", RiskAdvicePrefix, rel, m))
			} else if clientExts[ext] && isClientFile(rel, text) && strings.Contains(text, "service_role") {
				findings = append(findings, fmt.Sprintf("%s%s is browser code and mentions service_role. A service-role key bypasses every access rule; keep it on the server only.", RiskAdvicePrefix, rel))
			}
		}
		return nil
	})
	if usesSupabase {
		var open []string
		for t := range created {
			if !secured[t] && publicSchema(t) {
				open = append(open, t)
			}
		}
		sort.Strings(open)
		for _, t := range open {
			findings = append(findings, fmt.Sprintf("%stable %s (created in %s) has no row level security, so anyone holding the public key can read or change it. Add `alter table %s enable row level security;` and policies for who may do what.", RiskAdvicePrefix, t, created[t], t))
		}
	}
	sort.Strings(findings)
	return findings
}

func normalizeTable(t string) string {
	t = strings.ToLower(strings.ReplaceAll(t, `"`, ""))
	if !strings.Contains(t, ".") {
		t = "public." + t
	}
	return t
}

// publicSchema reports whether a table is in the schema a Supabase-style API exposes.
func publicSchema(t string) bool { return strings.HasPrefix(t, "public.") }

// isClientFile is a conservative guess that a file is shipped to the browser.
func isClientFile(rel, text string) bool {
	for _, dir := range []string{"public/", "static/", "src/components/", "components/", "src/pages/", "src/app/", "app/"} {
		if strings.HasPrefix(rel, dir) || strings.Contains(rel, "/"+dir) {
			if strings.HasPrefix(dir, "app/") || strings.HasSuffix(dir, "app/") {
				return firstLineIsUseClient(text)
			}
			return true
		}
	}
	return firstLineIsUseClient(text)
}

func firstLineIsUseClient(text string) bool {
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		return strings.HasPrefix(line, `"use client"`) || strings.HasPrefix(line, `'use client'`)
	}
	return false
}

// RecommendedReleaseSections are not required to record a release, but a product a non-technical
// owner runs alone needs them: a backup nobody restored is not a backup.
var RecommendedReleaseSections = map[string][]string{
	"Backups and restore": {"backups and restore", "backups e restauração", "copias de seguridad y restauración"},
	"Monitoring":          {"monitoring", "monitoramento", "monitorización"},
	"Free-tier limits":    {"free-tier limits", "limites do plano gratuito", "límites del plan gratuito"},
}

// ReleaseRecommendationPrefix starts each recommended-section advice line.
const ReleaseRecommendationPrefix = "Recommended before production: fill "

func (s *Service) releaseRecommendations() []string {
	path, e := safePath(s.Root, ReleasePlanPath)
	if e != nil {
		return nil
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil
	}
	sections := map[string]string{}
	current := ""
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "## ") {
			current = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			continue
		}
		if current != "" {
			sections[current] += line + "\n"
		}
	}
	names := make([]string, 0, len(RecommendedReleaseSections))
	for name := range RecommendedReleaseSections {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []string
	for _, name := range names {
		body, ok := findSection(sections, RecommendedReleaseSections[name])
		if !ok || !meaningful(body) {
			out = append(out, ReleaseRecommendationPrefix+"\""+name+"\" in "+ReleasePlanPath+".")
		}
	}
	return out
}
