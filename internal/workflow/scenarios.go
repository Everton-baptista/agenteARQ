package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// A task's `checks` list says which commands cover it, not what behaviour those commands must
// show. Scenarios close that gap: each acceptance criterion becomes at least one given/when/then
// the owner can read and a test can name. Personas say who the product is for, so a scenario
// and an agentic user run can be judged against a real goal instead of the author's guess.
//
// Neither file is born blocking. Absent, they are advice; present, they are a contract and are
// checked before planned, because a malformed contract is worse than none: it looks like
// coverage and is not.

const ScenariosPath = "agentarch/project/scenarios.yaml"
const PersonasPath = "agentarch/project/personas.yaml"

type Scenario struct {
	ID         string   `json:"id" yaml:"id"`
	Acceptance []string `json:"acceptance" yaml:"acceptance"`
	Given      string   `json:"given" yaml:"given"`
	When       string   `json:"when" yaml:"when"`
	Then       string   `json:"then" yaml:"then"`
	Kind       string   `json:"kind" yaml:"kind"`
}

type ScenarioDocument struct {
	SchemaVersion string     `json:"schema_version" yaml:"schema_version"`
	Scenarios     []Scenario `json:"scenarios" yaml:"scenarios"`
}

type Persona struct {
	ID                 string   `json:"id" yaml:"id"`
	Goal               string   `json:"goal" yaml:"goal"`
	Familiarity        string   `json:"familiarity" yaml:"familiarity"`
	Device             string   `json:"device" yaml:"device"`
	AccessibilityNeeds []string `json:"accessibility_needs,omitempty" yaml:"accessibility_needs,omitempty"`
	Adversarial        bool     `json:"adversarial,omitempty" yaml:"adversarial,omitempty"`
}

type PersonaDocument struct {
	SchemaVersion string    `json:"schema_version" yaml:"schema_version"`
	Personas      []Persona `json:"personas" yaml:"personas"`
}

var scenarioKinds = []string{"happy", "failure", "permission", "edge"}
var familiarities = []string{"none", "some", "expert"}
var criterionInText = regexp.MustCompile(`\bAC-[0-9]+\b`)

// AdviceScenarios and the other advice sentences are constants so the CLI can translate them
// by exact match, as it does the next-step actions.
const (
	AdviceScenarios = "Map each acceptance criterion to given/when/then scenarios in " + ScenariosPath + "; this becomes required in content 1.5."
	AdvicePersonas  = "Describe who the product is for in " + PersonasPath + " (goal, familiarity, device, accessibility needs), including one adversarial user."
	AdviceDesign    = "Before building the interface, record its direction in " + DesignBriefPath + " and its tokens in " + DesignTokensPath + " (W3C DTCG), so it looks like this product and not a default."
	AdviceAccept    = "Ask the owner to try the product and record it with workflow accept."
	// AdviceADRSources is given while an ADR records a decision without the date and sources of
	// the facts it used or the monthly cost the owner approved; market facts go stale fast.
	AdviceADRSources = "Add a \"## Sources and cost\" section to each ADR in " + ADRDir + ": the date and source of every price or limit used, and the estimated monthly cost the owner approved."
)

// uiProfiles are the profiles whose product has an interface a person looks at.
var uiProfiles = map[string]bool{"typescript-web": true, "static-site": true}

func (s *Service) exists(rel string) bool {
	p, e := safePath(s.Root, rel)
	if e != nil {
		return false
	}
	_, e = os.Stat(p)
	return e == nil
}

// specCriteria lists the acceptance criteria named in spec.md, in order of first appearance.
func specCriteria(spec []byte) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range criterionInText.FindAll(spec, -1) {
		if id := string(m); !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (s *Service) loadScenarios() (ScenarioDocument, error) {
	var d ScenarioDocument
	if e := readYAML(s.Root, ScenariosPath, &d); e != nil {
		return d, fmt.Errorf("%s: %w", ScenariosPath, e)
	}
	if d.SchemaVersion != SchemaVersion {
		return d, fmt.Errorf("%s: schema_version must be %q", ScenariosPath, SchemaVersion)
	}
	return d, nil
}

// scenarioGaps checks scenarios.yaml against the specification. Absent, it reports nothing.
func (s *Service) scenarioGaps(spec []byte) []string {
	if !s.exists(ScenariosPath) {
		return nil
	}
	d, e := s.loadScenarios()
	if e != nil {
		return []string{e.Error()}
	}
	var gaps []string
	inSpec := map[string]bool{}
	for _, c := range specCriteria(spec) {
		inSpec[c] = true
	}
	covered := map[string]bool{}
	ids := map[string]bool{}
	if len(d.Scenarios) == 0 {
		gaps = append(gaps, ScenariosPath+" must list at least one scenario")
	}
	for i, sc := range d.Scenarios {
		name := sc.ID
		if !slug.MatchString(sc.ID) {
			name = fmt.Sprintf("#%d", i+1)
			gaps = append(gaps, fmt.Sprintf("scenario %s needs a lowercase id", name))
		} else if ids[sc.ID] {
			gaps = append(gaps, "duplicate scenario id "+sc.ID)
		}
		ids[sc.ID] = true
		if !contains(scenarioKinds, sc.Kind) {
			gaps = append(gaps, "scenario "+name+" kind must be one of "+strings.Join(scenarioKinds, ", "))
		}
		for field, v := range map[string]string{"given": sc.Given, "when": sc.When, "then": sc.Then} {
			if strings.TrimSpace(v) == "" || strings.Contains(strings.ToUpper(v), "TODO") {
				gaps = append(gaps, "scenario "+name+" needs a concrete "+field)
			}
		}
		if len(sc.Acceptance) == 0 {
			gaps = append(gaps, "scenario "+name+" must name the acceptance criteria it demonstrates")
		}
		for _, a := range sc.Acceptance {
			if !criterion.MatchString(a) || !inSpec[a] {
				gaps = append(gaps, "scenario "+name+" references missing acceptance criterion "+a)
				continue
			}
			covered[a] = true
		}
	}
	for _, c := range specCriteria(spec) {
		if !covered[c] {
			gaps = append(gaps, c+" has no scenario in "+ScenariosPath)
		}
	}
	return sortedGaps(gaps)
}

// personaGaps checks personas.yaml when it exists.
func (s *Service) personaGaps() []string {
	if !s.exists(PersonasPath) {
		return nil
	}
	var d PersonaDocument
	if e := readYAML(s.Root, PersonasPath, &d); e != nil {
		return []string{PersonasPath + ": " + e.Error()}
	}
	var gaps []string
	if d.SchemaVersion != SchemaVersion {
		gaps = append(gaps, fmt.Sprintf("%s: schema_version must be %q", PersonasPath, SchemaVersion))
	}
	if len(d.Personas) == 0 {
		gaps = append(gaps, PersonasPath+" must describe at least one persona")
	}
	ids := map[string]bool{}
	for i, p := range d.Personas {
		name := p.ID
		if !slug.MatchString(p.ID) {
			name = fmt.Sprintf("#%d", i+1)
			gaps = append(gaps, fmt.Sprintf("persona %s needs a lowercase id", name))
		} else if ids[p.ID] {
			gaps = append(gaps, "duplicate persona id "+p.ID)
		}
		ids[p.ID] = true
		if !meaningful(p.Goal) {
			gaps = append(gaps, "persona "+name+" needs a concrete goal")
		}
		if !contains(familiarities, p.Familiarity) {
			gaps = append(gaps, "persona "+name+" familiarity must be one of "+strings.Join(familiarities, ", "))
		}
		if strings.TrimSpace(p.Device) == "" {
			gaps = append(gaps, "persona "+name+" needs a device")
		}
	}
	return gaps
}

// sortedGaps keeps map-iteration order out of user-facing output and tests.
func sortedGaps(g []string) []string {
	out := append([]string(nil), g...)
	sort.Strings(out)
	return out
}

// planningAdvice is what a product will want before planned but is not yet required.
func (s *Service) planningAdvice(p Product) []string {
	var advice []string
	if s.adrLacksSources() {
		advice = append(advice, AdviceADRSources)
	}
	if !s.exists(ScenariosPath) {
		advice = append(advice, AdviceScenarios)
	}
	if uiProfiles[p.Profile] {
		if !s.exists(PersonasPath) {
			advice = append(advice, AdvicePersonas)
		}
		if !s.exists(DesignBriefPath) || !s.exists(DesignTokensPath) {
			advice = append(advice, AdviceDesign)
		}
	}
	return advice
}

// adrLacksSources reports whether any ADR has no sources-and-cost section (any language).
func (s *Service) adrLacksSources() bool {
	entries, err := os.ReadDir(filepath.Join(s.Root, filepath.FromSlash(ADRDir)))
	if err != nil {
		return false
	}
	heading := regexp.MustCompile(`(?im)^##\s+(sources|fontes|fuentes)\b`)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(ADRDir), e.Name()))
		if err == nil && !heading.Match(b) {
			return true
		}
	}
	return false
}
