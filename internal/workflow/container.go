package workflow

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ContainerRunner deliberately has no local-execution fallback. The image must already
// exist locally, include dependencies and be pinned. No network, credentials, Git metadata
// or host socket is mounted into the container. Only an in-container, bounded tmpfs source copy is writable.
type ContainerRunner struct {
	Image             string
	DiagnosticsReason string // Explicit opt-in for bounded, redacted test output only.
}

var imageDigest = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}$`)

func (r *ContainerRunner) Isolation() string { return "oci:" + r.Image }
func (r *ContainerRunner) Run(ctx context.Context, root string, c Command) CheckResult {
	result := CheckResult{ID: c.ID, Subject: "product", Status: "inconclusive", ExitCode: -1}
	if err := ValidateDiagnosticsReason(r.DiagnosticsReason); err != nil {
		result.Message = err.Error()
		return result
	}
	if !imageDigest.MatchString(r.Image) {
		result.Message = "Configure an installed OCI verifier image pinned by @sha256 digest; local execution is disabled."
		return result
	}
	runtime := ""
	for _, name := range []string{"podman", "docker"} {
		if p, e := exec.LookPath(name); e == nil {
			runtime = p
			break
		}
	}
	if runtime == "" {
		result.Message = "Install Podman or Docker to run isolated checks; no local fallback was used."
		return result
	}
	service, e := New(root)
	if e != nil {
		result.Message = "Cannot resolve the source worktree."
		return result
	}
	stage, e := os.MkdirTemp("", "agentarch-verify-*")
	if e != nil {
		result.Message = "Cannot create isolated source staging directory."
		return result
	}
	defer os.RemoveAll(stage)
	source := filepath.Join(stage, "source")
	if e = os.Mkdir(source, 0700); e != nil {
		return result
	}
	if e = service.copySource(source); e != nil {
		return sourceStagingFailure(result, e)
	}
	archive, e := sourceArchive(source, filepath.Join(stage, "source.tar"))
	if e != nil {
		result.Message = "Cannot package the isolated source snapshot."
		return result
	}
	defer archive.Close()
	cid := filepath.Join(stage, "container-id")
	rootHash := sha256.Sum256([]byte(root))
	args := containerArgs(r.Image, source, cid, hex.EncodeToString(rootHash[:]), c)
	limit := c.Timeout
	if limit <= 0 || limit > 10*time.Minute {
		limit = 2 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(runCtx, runtime, args...)
	cmd.Stdin = archive
	sink := newDigestWriter(strings.TrimSpace(r.DiagnosticsReason) != "")
	cmd.Stdout = sink
	cmd.Stderr = sink
	started := time.Now()
	e = cmd.Run()
	result.DurationMS = time.Since(started).Milliseconds()
	result.OutputSHA256, result.OutputBytes, result.OutputExcerpt, result.OutputTruncated = sink.Snapshot()
	// Killing the client alone does not reliably stop the container daemon's process.
	if raw, err := os.ReadFile(cid); err == nil {
		id := strings.TrimSpace(string(raw))
		if regexp.MustCompile(`^[a-f0-9]{12,64}$`).MatchString(id) {
			cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			exec.CommandContext(cleanupCtx, runtime, "rm", "-f", id).Run()
			stop()
		}
	}
	if runCtx.Err() != nil {
		result.Message = "Check timed out or was cancelled; its container was removed."
		return result
	}
	if e == nil {
		result.Status = "passed"
		result.ExitCode = 0
		return result
	}
	if exit, ok := e.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
		if result.ExitCode != 125 && result.ExitCode != 126 && result.ExitCode != 127 {
			result.Status = "failed"
			result.Message = "Check returned a nonzero exit code."
		} else {
			result.Message = "Container or check executable is unavailable; prepare the pinned image with dependencies."
		}
	} else {
		result.Message = "Isolated verifier could not start."
	}
	return result
}

func sourceStagingFailure(result CheckResult, err error) CheckResult {
	result.Message = "Cannot stage source safely: " + err.Error()
	var invalid *SourceValidationError
	if errors.As(err, &invalid) {
		result.Status = "failed"
	}
	return result
}

func (s *Service) copySource(dest string) error {
	files, e := s.sourceFiles()
	if e != nil {
		return e
	}
	for _, rel := range files {
		if _, e := safePath(s.Root, filepath.Dir(rel)); e != nil {
			return e
		}
		in := filepath.Join(s.Root, rel)
		st, e := os.Lstat(in)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		out := filepath.Join(dest, rel)
		if e = os.MkdirAll(filepath.Dir(out), 0700); e != nil {
			return e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			target, e := os.Readlink(in)
			if e != nil {
				return e
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(rel), target))
			if filepath.IsAbs(target) || resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
				return invalidSource("symlink %s escapes its source tree", rel)
			}
			if e = os.Symlink(target, out); e != nil {
				return e
			}
			continue
		}
		if !st.Mode().IsRegular() {
			return invalidSource("unsupported source type: %s", rel)
		}
		reader, e := os.Open(in)
		if e != nil {
			return e
		}
		writer, e := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, st.Mode().Perm())
		if e != nil {
			reader.Close()
			return e
		}
		_, e = io.Copy(writer, reader)
		reader.Close()
		closeErr := writer.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

// The wrapper is fixed code. Untrusted program and arguments are positional values in
// "$@", never interpolated into shell source. The snapshot is streamed through stdin:
// no host directory is mounted and remote daemons need not share the caller's filesystem.
func containerArgs(image, source, cid, projectID string, c Command) []string {
	args := []string{"run", "--rm", "--interactive", "--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=128", "--memory=1g", "--cpus=2", "--cidfile", cid, "--label", "agentarch.workflow.root=" + projectID, "--tmpfs", "/tmp:rw,nosuid,nodev,size=128m", "--tmpfs", "/workspace:rw,nosuid,nodev,size=512m", "--workdir", "/workspace", "--env", "HOME=/tmp", "--env", "CI=true", "--entrypoint", "/bin/sh", image, "-c", `tar --no-same-owner -xf - -C /workspace && exec "$@"`, "agentarch-verifier", c.Program}
	return append(args, c.Args...)
}

func sourceArchive(source, dest string) (*os.File, error) {
	f, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	tw := tar.NewWriter(f)
	e = filepath.WalkDir(source, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == source {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, p)
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(p)
			if err != nil {
				return err
			}
		}
		h, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		// A snapshot carries source bytes/modes, never the host's account ownership.
		// Restoring a developer UID as container root requires CAP_CHOWN, which the
		// verifier deliberately drops. Normalize metadata instead of granting it.
		h.Uid, h.Gid = 0, 0
		h.Uname, h.Gname = "", ""
		if err = tw.WriteHeader(h); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		r, err := os.Open(p)
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, r)
		r.Close()
		return err
	})
	closeErr := tw.Close()
	if e == nil {
		e = closeErr
	}
	if e == nil {
		_, e = f.Seek(0, io.SeekStart)
	}
	if e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
