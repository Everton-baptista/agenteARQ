package workflow

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNextSelectsEligibleTaskAndUnblocksDependencies(t *testing.T) {
	s, _ := fixture(t)
	for _, rel := range documents[:2] {
		write(t, s.Root, rel, "# Product artifact\n\nAC-1: Only authorized customers can find their orders.\n")
	}
	write(t, s.Root, documents[2], testADR)
	tasks := []Task{
		{ID: "release", DependsOn: []string{"test", "docs"}},
		{ID: "test", DependsOn: []string{"build"}},
		{ID: "build"},
		{ID: "docs"},
	}
	for i := range tasks {
		tasks[i].Title = "Complete " + tasks[i].ID
		tasks[i].Acceptance = []string{"AC-1"}
		tasks[i].Checks = []string{"test"}
	}
	b, err := yaml.Marshal(TaskDocument{SchemaVersion: SchemaVersion, Tasks: tasks})
	if err != nil {
		t.Fatal(err)
	}
	write(t, s.Root, TasksPath, string(b))
	for _, phase := range []string{"specified", "planned", "implementing"} {
		if _, err := s.Checkpoint(CheckpointRequest{Phase: phase}); err != nil {
			t.Fatal(err)
		}
	}
	steps := []struct {
		complete string
		next     string
		ready    []string
		blocked  []string
	}{
		{"", "build", []string{"build", "docs"}, []string{"release", "test"}},
		{"build", "test", []string{"test", "docs"}, []string{"release"}},
		{"test", "docs", []string{"docs"}, []string{"release"}},
		{"docs", "release", []string{"release"}, nil},
		{"release", "", nil, nil},
	}
	for _, step := range steps {
		if step.complete != "" {
			if _, err := s.Checkpoint(CheckpointRequest{Phase: "implementing", TaskID: step.complete}); err != nil {
				t.Fatal(err)
			}
		}
		next, err := s.Next()
		if err != nil {
			t.Fatal(err)
		}
		if next.TaskID != step.next || !reflect.DeepEqual(next.ReadyTaskIDs, step.ready) || !reflect.DeepEqual(next.BlockedTaskIDs, step.blocked) {
			t.Fatalf("after completing %q: got %+v, expected task=%q ready=%v blocked=%v", step.complete, next, step.next, step.ready, step.blocked)
		}
		if step.complete == "" && !strings.Contains(strings.Join(next.Missing, "; "), "task release depends on incomplete test, docs") {
			t.Fatalf("missing prerequisites: %+v", next)
		}
		if step.complete == "release" && (!strings.Contains(next.Action, "run verification") || len(next.Missing) != 0) {
			t.Fatalf("completed tasks still suggested: %+v", next)
		}
	}
}
