package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestDigestCoversOutputAfterFirstMiBWithoutCapturingContent(t *testing.T) {
	prefix := strings.Repeat("x", 1024*1024+7)
	w := newDigestWriter(false)
	w.Write([]byte(prefix))
	w.Write([]byte("distinct tail"))
	want := sha256.Sum256([]byte(prefix + "distinct tail"))
	digest, bytes, excerpt, truncated := w.Snapshot()
	if digest != hex.EncodeToString(want[:]) || bytes != int64(len(prefix)+len("distinct tail")) || excerpt != "" || truncated || len(w.buf) != 0 {
		t.Fatalf("default capture leaked or digest incomplete: digest=%s bytes=%d excerpt=%q truncated=%v", digest, bytes, excerpt, truncated)
	}
}

func TestDiagnosticsRedactBeforeReturningBoundedExcerpt(t *testing.T) {
	w := newDigestWriter(true)
	text := "\x1b[31mCompile failed\x1b[0m\napi_key=sk-abcdefghijklmnopqrstuv\nAuthorization: Bearer abcdefghijklmnopqrstuv\npassword=not-for-output\n-----BEGIN PRIVATE KEY-----\nnot-for-output\n-----END PRIVATE KEY-----\n"
	// Split tokens across Write calls to exercise redaction after stream assembly.
	w.Write([]byte(text[:37]))
	w.Write([]byte(text[37:]))
	_, bytes, excerpt, truncated := w.Snapshot()
	if bytes != int64(len(text)) || truncated || !strings.Contains(excerpt, "Compile failed") || !strings.Contains(excerpt, "[REDACTED]") {
		t.Fatalf("diagnostic = %q, bytes=%d truncated=%v", excerpt, bytes, truncated)
	}
	for _, forbidden := range []string{"sk-abcdefghijkl", "abcdefghijklmnopqrstuv", "not-for-output", "PRIVATE KEY", "\x1b"} {
		if strings.Contains(excerpt, forbidden) {
			t.Fatalf("diagnostic retained %q", forbidden)
		}
	}
}

func TestDiagnosticBoundsAndTruncationDoNotTruncateDigest(t *testing.T) {
	w := newDigestWriter(true)
	text := strings.Repeat("Useful compiler diagnostic line\n", 4000)
	w.Write([]byte(text))
	digest, bytes, excerpt, truncated := w.Snapshot()
	want := sha256.Sum256([]byte(text))
	if digest != hex.EncodeToString(want[:]) || bytes != int64(len(text)) || len(excerpt) > diagnosticExcerptLimit || !truncated || !utf8.ValidString(excerpt) || len(w.buf) > diagnosticCaptureLimit {
		t.Fatalf("unbounded diagnostic: bytes=%d excerpt=%d captured=%d truncated=%v", bytes, len(excerpt), len(w.buf), truncated)
	}
	long := newDigestWriter(true)
	long.Write([]byte("token=" + strings.Repeat("secret", diagnosticCaptureLimit)))
	_, _, excerpt, truncated = long.Snapshot()
	if !truncated || strings.Contains(excerpt, "secret") {
		t.Fatalf("partial secret line retained: %q", excerpt)
	}
}

func TestDigestConcurrentWritersRemainBounded(t *testing.T) {
	w := newDigestWriter(false)
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 100; j++ {
				w.Write([]byte("x"))
			}
		}()
	}
	group.Wait()
	want := sha256.Sum256([]byte(strings.Repeat("x", 800)))
	if w.Digest() != hex.EncodeToString(want[:]) {
		t.Fatal("concurrent output digest is incomplete")
	}
}

func TestDiagnosticsReasonAndStateSchema(t *testing.T) {
	for _, valid := range []string{"", "Synthetic fixture compiler diagnostics required for repair."} {
		if err := ValidateDiagnosticsReason(valid); err != nil {
			t.Fatal(err)
		}
	}
	for _, invalid := range []string{"short", strings.Repeat("x", 501), "capture\nsynthetic outputs", "secret=abcdefghijklmnop"} {
		if ValidateDiagnosticsReason(invalid) == nil {
			t.Fatalf("accepted invalid reason %q", invalid)
		}
	}
	state := State{SchemaVersion: SchemaVersion, Revision: 1, Phase: "verifying", Worktree: "/fixture", CompletedTasks: []string{}, Checkpoints: []CheckpointRecord{}, Verification: &Verification{Fingerprint: Fingerprint{Worktree: "/fixture", SourceSHA256: strings.Repeat("a", 64), Profile: "python-api"}, Isolation: "oci:fixture", Checks: []CheckResult{{ID: "lint", Subject: "product", Status: "failed", ExitCode: 1, OutputBytes: 500, OutputExcerpt: "Compiler error [REDACTED]", OutputTruncated: true}}, DiagnosticsReason: "Synthetic fixture output needed for repair."}}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateArtifact(raw, "workflow.state.schema.json"); err != nil {
		t.Fatal(err)
	}
}
