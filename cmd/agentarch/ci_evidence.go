package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// gateRunsInCI verifies configuration, not a remote CI execution or branch protection.
// Unknown shell syntax/CI vendors earn no evidence instead of a guessed success.
func gateRunsInCI(root string) bool {
	files, _ := filepath.Glob(filepath.Join(root, ".github", "workflows", "*"))
	for _, path := range files {
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc struct {
			Defaults struct {
				Run struct {
					Shell string `yaml:"shell"`
				} `yaml:"run"`
			} `yaml:"defaults"`
			Jobs map[string]struct {
				Defaults struct {
					Run struct {
						Shell string `yaml:"shell"`
					} `yaml:"run"`
				} `yaml:"defaults"`
				Continue any    `yaml:"continue-on-error"`
				If       string `yaml:"if"`
				Steps    []struct {
					Run      string `yaml:"run"`
					Continue any    `yaml:"continue-on-error"`
					If       string `yaml:"if"`
					Shell    string `yaml:"shell"`
				} `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if yaml.Unmarshal(b, &doc) != nil {
			continue
		}
		for _, job := range doc.Jobs {
			if (job.Continue != nil && job.Continue != false) || job.If != "" {
				continue
			}
			for _, step := range job.Steps {
				shell := step.Shell
				if shell == "" {
					shell = job.Defaults.Run.Shell
				}
				if shell == "" {
					shell = doc.Defaults.Run.Shell
				}
				if shell != "" && shell != "bash" && shell != "sh" {
					continue
				}
				if (step.Continue != nil && step.Continue != false) || step.If != "" {
					continue
				}
				if blockingGateCommand(step.Run) {
					return true
				}
			}
		}
	}
	return false
}

var gateCommand = regexp.MustCompile(`^(?:[A-Za-z0-9_./-]+/)?agentarch\s+check(?:\s|$)`)

func blockingGateCommand(script string) bool {
	if strings.Contains(script, "set +e") || strings.Contains(script, "||") || strings.Contains(script, "exit 0") {
		return false
	}
	// Recognize straight-line invocations only. A command inside a function,
	// heredoc, conditional or loop is not evidence that the gate will run.
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsAny(line, ";&|`$<>{}()\\") {
			return false
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "if", "then", "else", "elif", "fi", "case", "esac", "for", "while", "until", "do", "done", "function", "trap", "exit", "return", "exec", "eval", "source", ".":
			return false
		case "set":
			if line != "set -e" && line != "set -eu" && line != "set -euo pipefail" {
				return false
			}
		}
	}
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if gateCommand.MatchString(line) && !strings.ContainsAny(line, ";&|`") {
			return true
		}
	}
	return false
}
