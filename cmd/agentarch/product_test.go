package main

import (
	"strings"
	"testing"
)

func TestProductCommandHelpDoesNotNeedProject(t *testing.T) {
	for _, args := range [][]string{{"workflow", "--help"}, {"workflow", "-h"}, {"workflow", "help"}, {"workflow", "record-pr", "--help"}, {"plugin", "--help"}, {"plugin", "-h"}, {"plugin", "help"}, {"plugin", "install", "--help"}} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			out, err, code := capture(t, func() int { return run(args) })
			if code != exitOK {
				t.Fatalf("help exited %d: %s %s", code, out, err)
			}
			if !strings.Contains(strings.ToLower(out+err), "usage") {
				t.Fatal("missing usage")
			}
			if args[0] == "workflow" && len(args) == 2 && !strings.Contains(out+err, "record-pr") {
				t.Fatal("PR registration missing from help")
			}
		})
	}
}
func TestProductCommandsRejectIgnoredArguments(t *testing.T) {
	for _, args := range [][]string{{"workflow", "status", "unexpected"}, {"plugin", "list", "--format", "xml"}, {"plugin", "check", "--host", "codex", "claude"}} {
		_, _, code := capture(t, func() int { return run(args) })
		if code != exitUsage {
			t.Fatalf("accepted ignored arguments: %v", args)
		}
	}
}
