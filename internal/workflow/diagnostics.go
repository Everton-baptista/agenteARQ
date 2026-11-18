package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const diagnosticCaptureLimit = 64 * 1024
const diagnosticExcerptLimit = 16 * 1024

var diagnosticANSI = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var diagnosticSecrets = []*regexp.Regexp{
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{12,}`),
	regexp.MustCompile(`AIza[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{16,}`),
	regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),
	regexp.MustCompile(`(?i)Bearer[ \t]+[A-Za-z0-9_.-]{12,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
}
var diagnosticAssignment = regexp.MustCompile(`(?im)(["']?(?:api[_-]?key|access[_-]?token|refresh[_-]?token|authorization|password|secret|token)["']?[ \t]*[:=][ \t]*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;\r\n]+)`)

// ValidateDiagnosticsReason leaves the empty/default configuration content-free.
// A nonempty reason is an explicit operator decision, not an assertion that the
// output cannot contain sensitive data. Recognizable credentials are rejected.
func ValidateDiagnosticsReason(reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil
	}
	if len(reason) < 10 || len(reason) > 500 || !utf8.ValidString(reason) || strings.IndexFunc(reason, unicode.IsControl) >= 0 {
		return errors.New("diagnostics reason must be 10..500 UTF-8 bytes without control characters")
	}
	if redactDiagnostic(reason) != reason {
		return errors.New("diagnostics reason contains credential-shaped content; describe the purpose without secrets")
	}
	return nil
}

// Redaction is deliberately described as best effort. It is not a general PII
// detector, and synthetic fixtures must not contain secrets or personal records.
func redactDiagnostic(text string) string {
	text = strings.ToValidUTF8(text, "\ufffd")
	text = diagnosticANSI.ReplaceAllString(text, "")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
	for _, pattern := range diagnosticSecrets {
		text = pattern.ReplaceAllString(text, "[REDACTED]")
	}
	return diagnosticAssignment.ReplaceAllString(text, "${1}[REDACTED]")
}

// Hash the complete merged stdout/stderr stream without retaining its content by
// default. Diagnostic capture is a bounded prefix in memory, never a raw log file.
type digestWriter struct {
	mu      sync.Mutex
	hash    hash.Hash
	bytes   int64
	capture bool
	buf     []byte
}

func newDigestWriter(capture bool) *digestWriter {
	return &digestWriter{hash: sha256.New(), capture: capture}
}

func (w *digestWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hash == nil {
		w.hash = sha256.New()
	}
	w.hash.Write(p)
	w.bytes += int64(len(p))
	if w.capture && len(w.buf) < diagnosticCaptureLimit {
		n := diagnosticCaptureLimit - len(w.buf)
		if n > len(p) {
			n = len(p)
		}
		w.buf = append(w.buf, p[:n]...)
	}
	return len(p), nil
}

func (w *digestWriter) Snapshot() (digest string, bytes int64, excerpt string, truncated bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hash == nil {
		w.hash = sha256.New()
	}
	digest = hex.EncodeToString(w.hash.Sum(nil))
	bytes = w.bytes
	if !w.capture {
		return
	}
	raw := string(w.buf)
	truncated = int64(len(w.buf)) < w.bytes
	if truncated {
		// Do not retain a partial final line that could split a credential pattern.
		if last := strings.LastIndexByte(raw, '\n'); last >= 0 {
			raw = raw[:last+1]
		} else {
			raw = "[diagnostic omitted: first line exceeded capture limit]"
		}
	}
	excerpt = redactDiagnostic(raw)
	if len(excerpt) > diagnosticExcerptLimit {
		excerpt = excerpt[:diagnosticExcerptLimit]
		for !utf8.ValidString(excerpt) {
			excerpt = excerpt[:len(excerpt)-1]
		}
		truncated = true
	}
	return
}

func (w *digestWriter) Digest() string {
	digest, _, _, _ := w.Snapshot()
	return digest
}
