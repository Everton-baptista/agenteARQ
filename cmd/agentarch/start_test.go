package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPersonalizeManifestsPreservesStructure(t *testing.T) {
	const hash = "09548c5bd8c196f8ea5012c815312051c70bb11792527981168cd661d1e123e3"
	for _, jurisdictionYAML := range []string{"  jurisdictions: [EU, BR]\n", "  jurisdictions:\n  - EU\n  - BR\n"} {
		for _, countryInput := range []string{" br, eu ", ""} {
			t.Run(jurisdictionYAML+countryInput, func(t *testing.T) {
				root := t.TempDir()
				dir := filepath.Join(root, "agentarch", "project", "agents", "fixture")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, "agent.yaml")
				input := "# Preserve the guidance\nschema_version: '1.0'\nmetadata:\n  accountable: other\n  jurisdictions: [CA]\nagent:\n  owner:\n    accountable: original # Responsible person\n    contact: original@example.com\n" + jurisdictionYAML +
					"  prompts:\n    system:\n      path: prompts/system.v1.1.md\n      version: 1.1.0\n      sha256: " + hash + "\n"
				if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
					t.Fatal(err)
				}
				owner := "Ana: Costa #2\nwith a second line"
				contact := "true"
				count, err := personalizeManifests(root, owner, contact, countryInput)
				if err != nil || count != 1 {
					t.Fatalf("personalize count=%d err=%v", count, err)
				}
				output, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var got struct {
					Metadata struct {
						Accountable   string   `yaml:"accountable"`
						Jurisdictions []string `yaml:"jurisdictions"`
					} `yaml:"metadata"`
					Agent struct {
						Owner struct {
							Accountable string `yaml:"accountable"`
							Contact     string `yaml:"contact"`
						} `yaml:"owner"`
						Jurisdictions []string `yaml:"jurisdictions"`
						Prompts       struct {
							System map[string]string `yaml:"system"`
						} `yaml:"prompts"`
					} `yaml:"agent"`
				}
				if err := yaml.Unmarshal(output, &got); err != nil {
					t.Fatalf("personalized YAML invalid: %v\n%s", err, output)
				}
				if got.Agent.Owner.Accountable != owner || got.Agent.Owner.Contact != contact {
					t.Fatalf("owner strings changed during YAML encoding: %#v", got.Agent.Owner)
				}
				wantCountries := []string{}
				if countryInput != "" {
					wantCountries = []string{"BR", "EU"}
				}
				if !reflect.DeepEqual(got.Agent.Jurisdictions, wantCountries) {
					t.Fatalf("countries = %v, want %v", got.Agent.Jurisdictions, wantCountries)
				}
				if got.Metadata.Accountable != "other" || !reflect.DeepEqual(got.Metadata.Jurisdictions, []string{"CA"}) {
					t.Fatal("modified a similarly named field outside agent")
				}
				wantPrompt := map[string]string{"path": "prompts/system.v1.1.md", "version": "1.1.0", "sha256": hash}
				if !reflect.DeepEqual(got.Agent.Prompts.System, wantPrompt) {
					t.Fatalf("prompt contract changed: %v", got.Agent.Prompts.System)
				}
				if !strings.Contains(string(output), "# Preserve the guidance") || !strings.Contains(string(output), "# Responsible person") {
					t.Fatal("manifest comments were lost")
				}
			})
		}
	}
}

func TestPersonalizeManifestsRejectsMalformedYAML(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "agentarch", "project", "agents", "fixture")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agent.yaml")
	input := []byte("agent: [broken\n")
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := personalizeManifests(root, "Fixture", "", "BR"); err == nil {
		t.Fatal("malformed YAML was accepted")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(input) {
		t.Fatal("malformed manifest was overwritten")
	}
}
