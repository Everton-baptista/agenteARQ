package main

import (
	"flag"
	"testing"
)

func TestDiagnosticsFlagRequiresExplicitPurposeAndDefaultsOff(t *testing.T) {
	for _, test := range []struct {
		args  []string
		valid bool
		want  string
	}{
		{nil, true, ""},
		{[]string{"--diagnostics-reason", ""}, false, ""},
		{[]string{"--diagnostics-reason", "short"}, false, ""},
		{[]string{"--diagnostics-reason", "Synthetic fixture compiler diagnostics required for repair."}, true, "Synthetic fixture compiler diagnostics required for repair."},
	} {
		flags := flag.NewFlagSet("test", flag.ContinueOnError)
		value := flags.String("diagnostics-reason", "", "")
		if err := flags.Parse(test.args); err != nil {
			t.Fatal(err)
		}
		got, err := diagnosticsFlagReason(flags, *value)
		if (err == nil) != test.valid || (test.valid && got != test.want) {
			t.Fatalf("args=%v got=%q err=%v", test.args, got, err)
		}
	}
	if code := cmdWorkflow([]string{"status", "--diagnostics-reason", "Synthetic fixture diagnostics only."}); code == 0 {
		t.Fatal("non-verification command accepted output capture")
	}
}
