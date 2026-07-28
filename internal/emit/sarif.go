// Package emit renders gate results for machines.
//
// SARIF exists so findings land in GitHub code scanning, annotated on the changed lines,
// instead of only in a CI log nobody opens after the build goes green.
package emit

import (
	"encoding/json"
	"io"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Version        string      `json:"version"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string            `json:"id"`
	Name             string            `json:"name,omitempty"`
	ShortDescription sarifText         `json:"shortDescription"`
	FullDescription  sarifText         `json:"fullDescription,omitempty"`
	Help             sarifText         `json:"help,omitempty"`
	Properties       map[string]any    `json:"properties,omitempty"`
	DefaultConfig    sarifRuleDefaults `json:"defaultConfiguration"`
}

type sarifRuleDefaults struct {
	Level string `json:"level"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifText       `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"`
	} `json:"physicalLocation"`
}

// level maps severity onto SARIF's vocabulary. Warn-mode controls report as "note" so a grace
// period is visible without turning the pull request red.
func level(s policy.Severity) string {
	switch s {
	case policy.SevBlocker:
		return "error"
	case policy.SevMajor:
		return "warning"
	case policy.SevMinor, policy.SevWarn:
		return "note"
	}
	return "none"
}

// SARIF writes gate results in SARIF 2.1.0.
func SARIF(w io.Writer, results []policy.Result, version string, fileFor func(policy.Result) string) error {
	rulesSeen := map[string]bool{}
	var rules []sarifRule
	var out []sarifResult

	for _, r := range results {
		if r.Passed && r.Error == "" {
			continue
		}
		if !rulesSeen[r.ControlID] {
			rulesSeen[r.ControlID] = true
			help := r.Remediation
			if r.StandardRef != "" {
				help += "\n\nStandard: " + r.StandardRef
			}
			rules = append(rules, sarifRule{
				ID:               r.ControlID,
				Name:             r.ControlID,
				ShortDescription: sarifText{Text: r.Title},
				FullDescription:  sarifText{Text: r.Message},
				Help:             sarifText{Text: help},
				DefaultConfig:    sarifRuleDefaults{Level: level(r.Severity)},
				Properties: map[string]any{
					"pack":     r.FromPack,
					"severity": string(r.Severity),
					"evidence": r.Evidence,
				},
			})
		}

		msg := r.Message
		if r.Error != "" {
			msg = "control could not be evaluated: " + r.Error
		}
		if msg == "" {
			msg = r.Title
		}
		msg += "  [" + r.FromPack + "@" + r.PackVersion + "]"
		if r.Waived {
			msg += "  (waived until " + r.WaiverUntil + " by " + r.WaiverOwner + ")"
		}
		if r.Remediation != "" {
			msg += "\nFix: " + r.Remediation
		}

		lvl := level(r.Severity)
		if r.Waived {
			lvl = "note"
		}

		res := sarifResult{RuleID: r.ControlID, Level: lvl, Message: sarifText{Text: msg}}
		var loc sarifLocation
		loc.PhysicalLocation.ArtifactLocation.URI = fileFor(r)
		res.Locations = []sarifLocation{loc}
		out = append(out, res)
	}

	if out == nil {
		out = []sarifResult{}
	}
	if rules == nil {
		rules = []sarifRule{}
	}

	log := sarifLog{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "agentarch",
				InformationURI: "https://github.com/Everton-baptista/agenteARQ",
				Version:        version,
				Rules:          rules,
			}},
			Results: out,
		}},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}
