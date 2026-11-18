package workflow

import (
	"context"
	"errors"
	"time"
)

// VerifyProfile measures fixed engineering checks without installing a plugin or
// creating workflow state. It makes no claim about agent policy or review approval.
// Effect: write to bounded disposable container snapshots, never the source tree.
func VerifyProfile(ctx context.Context, root, profile, image string) (Verification, error) {
	return VerifyProfileWithDiagnostics(ctx, root, profile, image, "")
}

// VerifyProfileWithDiagnostics explicitly opts into bounded, redacted test output.
// It never captures model conversations, changes the source, or grants additional permissions.
func VerifyProfileWithDiagnostics(ctx context.Context, root, profile, image, reason string) (Verification, error) {
	if !imageDigest.MatchString(image) {
		return Verification{}, errors.New("prepared verifier image must be pinned by @sha256 repository digest")
	}
	if err := ValidateDiagnosticsReason(reason); err != nil {
		return Verification{}, err
	}
	v, err := verifyProfile(ctx, root, profile, &ContainerRunner{Image: image, DiagnosticsReason: reason})
	v.DiagnosticsReason = reason
	return v, err
}

func verifyProfile(ctx context.Context, root, profile string, runner Runner) (Verification, error) {
	commands, err := ProfileCommands(profile)
	if err != nil {
		return Verification{}, err
	}
	if len(commands) == 0 {
		return Verification{}, errors.New("verify-profile requires a fixed engineering profile")
	}
	s, err := New(root)
	if err != nil {
		return Verification{}, err
	}
	fingerprint, err := s.fingerprint(profile)
	if err != nil {
		return Verification{}, err
	}
	v := Verification{Fingerprint: fingerprint, StartedAt: time.Now().UTC(), Isolation: runner.Isolation(), Checks: []CheckResult{}, Passed: true}
	for _, command := range commands {
		check := runner.Run(ctx, s.Root, command)
		v.Checks = append(v.Checks, check)
		if check.Status != "passed" {
			v.Passed = false
		}
		if ctx.Err() != nil {
			v.Passed = false
			break
		}
	}
	after, err := s.fingerprint(profile)
	if err != nil || after != fingerprint {
		v.Passed = false
		v.Checks = append(v.Checks, CheckResult{ID: "source-stability", Subject: "product", Status: "inconclusive", ExitCode: -1, Message: "Source changed or became unavailable during verification."})
	}
	v.FinishedAt = time.Now().UTC()
	return v, nil
}
