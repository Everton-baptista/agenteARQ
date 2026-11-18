package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Service struct {
	Root   string
	gitDir string
}

func New(root string) (*Service, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	top, err := git(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, errors.New("workflow requires a Git repository; initialize Git in the project first")
	}
	top, err = filepath.EvalSymlinks(strings.TrimSpace(top))
	if err != nil {
		return nil, err
	}
	if abs != top {
		return nil, errors.New("workflow root must be the Git worktree root")
	}
	gd, err := git(abs, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	return &Service{Root: abs, gitDir: strings.TrimSpace(gd)}, nil
}

func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "core.fsmonitor=false"}, args...)...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w", args[0], err)
	}
	return string(out), nil
}

func (s *Service) localDir() string  { return filepath.Join(s.gitDir, "agentarch-workflow") }
func (s *Service) statePath() string { return filepath.Join(s.localDir(), "state.json") }

func safePath(root, rel string) (string, error) {
	if filepath.IsAbs(rel) || filepath.Clean(rel) == ".." || strings.HasPrefix(filepath.Clean(rel), ".."+string(filepath.Separator)) {
		return "", invalidSource("path escapes project")
	}
	p := filepath.Join(root, rel)
	// Artifacts and directories cannot be redirected through a symlink.
	cur := root
	for _, part := range strings.Split(filepath.Clean(rel), string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		st, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return "", invalidSource("symlink is not allowed in artifact path: %s", rel)
		}
	}
	return p, nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".agentarch-write-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	// Flush directory metadata where supported; atomic rename already protects readers.
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *Service) save(st State) error {
	st.Revision++
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(s.statePath(), append(b, '\n'), 0600); err != nil {
		return err
	}
	if st.Verification != nil {
		return s.exportEvidence(st)
	}
	return nil
}
func (s *Service) loadState() (State, error) {
	var st State
	b, e := os.ReadFile(s.statePath())
	if e != nil {
		return st, fmt.Errorf("workflow state missing; run workflow init: %w", e)
	}
	if e := validateArtifact(b, "workflow.state.schema.json"); e != nil {
		return st, e
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	e = d.Decode(&st)
	if e == nil && (st.SchemaVersion != SchemaVersion || st.Worktree != s.Root) {
		e = errors.New("workflow state belongs to a different schema or worktree")
	}
	return st, e
}
func readYAML(root, rel string, dest any) error {
	p, e := safePath(root, rel)
	if e != nil {
		return e
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	schemaName := ""
	if rel == ProductPath {
		schemaName = "product.schema.json"
	}
	if rel == TasksPath {
		schemaName = "workflow.tasks.schema.json"
	}
	if schemaName != "" {
		if e := validateArtifact(b, schemaName); e != nil {
			return e
		}
	}
	d := yaml.NewDecoder(strings.NewReader(string(b)))
	d.KnownFields(true)
	if e = d.Decode(dest); e != nil {
		return e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return errors.New("artifact must contain exactly one YAML document")
	}
	return nil
}
func (s *Service) loadArtifacts() (Product, TaskDocument, error) {
	var p Product
	var tasks TaskDocument
	if e := readYAML(s.Root, ProductPath, &p); e != nil {
		return p, tasks, e
	}
	if p.SchemaVersion != SchemaVersion {
		return p, tasks, errors.New("unsupported product schema_version")
	}
	if _, ok := profiles[p.Profile]; !ok {
		return p, tasks, fmt.Errorf("unknown engineering profile %q", p.Profile)
	}
	if e := readYAML(s.Root, TasksPath, &tasks); e != nil {
		return p, tasks, e
	}
	if tasks.SchemaVersion != SchemaVersion {
		return p, tasks, errors.New("unsupported tasks schema_version")
	}
	return p, tasks, validateTasks(tasks.Tasks)
}

func (s *Service) sourceFiles() ([]string, error) {
	out, e := git(s.Root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if e != nil {
		return nil, e
	}
	seen := map[string]bool{}
	files := []string{}
	for _, p := range strings.Split(out, "\x00") {
		if p == "" || strings.HasPrefix(p, "agentarch/project/evidence/") {
			continue
		}
		if filepath.IsAbs(p) || filepath.Clean(p) == ".." || strings.HasPrefix(filepath.Clean(p), ".."+string(filepath.Separator)) {
			return nil, invalidSource("unsafe Git source path")
		}
		if !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	sort.Strings(files)
	return files, nil
}

func (s *Service) fingerprint(profile string) (Fingerprint, error) {
	f := Fingerprint{Worktree: s.Root, GitDir: s.gitDir, Profile: profile}
	f.Head, _ = git(s.Root, "rev-parse", "--verify", "HEAD")
	f.Head = strings.TrimSpace(f.Head)
	if f.Head == "" {
		f.Head = "unborn"
	}
	f.Branch, _ = git(s.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
	f.Branch = strings.TrimSpace(f.Branch)
	files, e := s.sourceFiles()
	if e != nil {
		return f, e
	}
	h := sha256.New()
	for _, rel := range files {
		if _, e := safePath(s.Root, filepath.Dir(rel)); e != nil {
			return f, e
		}
		p := filepath.Join(s.Root, rel)
		st, e := os.Lstat(p)
		fmt.Fprintf(h, "%s\x00", rel)
		if os.IsNotExist(e) {
			io.WriteString(h, "deleted\x00")
			continue
		}
		if e != nil {
			return f, e
		}
		fmt.Fprintf(h, "%v\x00", st.Mode())
		if st.Mode()&os.ModeSymlink != 0 {
			target, e := os.Readlink(p)
			if e != nil {
				return f, e
			}
			io.WriteString(h, target)
		} else if st.Mode().IsRegular() {
			file, e := os.Open(p)
			if e != nil {
				return f, e
			}
			_, e = io.Copy(h, file)
			file.Close()
			if e != nil {
				return f, e
			}
		} else {
			return f, invalidSource("unsupported source file type: %s", rel)
		}
		h.Write([]byte{0})
	}
	f.SourceSHA256 = hex.EncodeToString(h.Sum(nil))
	return f, nil
}
func sameFingerprint(a, b Fingerprint) bool { return a == b }
func revisionOK(st State, want *uint64) error {
	if want != nil && st.Revision != *want {
		return fmt.Errorf("revision conflict: expected %d, current %d", *want, st.Revision)
	}
	return nil
}
func now() time.Time { return time.Now().UTC() }
