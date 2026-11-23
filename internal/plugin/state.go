package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type entry struct {
	Kind         string `json:"kind"`
	Hash         string `json:"sha256"`
	Preserve     bool   `json:"preserve,omitempty"`
	Existed      bool   `json:"existed,omitempty"`
	AddedNewline bool   `json:"added_newline,omitempty"`
}
type state struct {
	Version int              `json:"version"`
	Host    string           `json:"host"`
	Entries map[string]entry `json:"entries"`
}
type mutation struct {
	Path            string
	Data            []byte
	Delete, Private bool
}
type snapshot struct {
	data   []byte
	exists bool
	mode   os.FileMode
}

func stateFile(host string) string { return ".agentarch/plugin-state/" + host + "/state.json" }

func canonicalRoot(root string) (string, error) {
	a, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	a, err = filepath.EvalSymlinks(a)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(a)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", errors.New("project root is not a directory")
	}
	return a, nil
}

func safePath(root, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || filepath.ToSlash(filepath.Clean(rel)) != rel || strings.HasPrefix(rel, "../") || rel == ".." || rel == "." || strings.ContainsRune(rel, '\x00') {
		return "", fmt.Errorf("unsafe plugin path %q", rel)
	}
	p := root
	for _, part := range strings.Split(rel, "/") {
		p = filepath.Join(p, part)
		st, err := os.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("plugin path contains a symlink: %s", rel)
		}
	}
	return p, nil
}

func readSafe(root, rel string) ([]byte, bool, error) {
	p, err := safePath(root, rel)
	if err != nil {
		return nil, false, err
	}
	st, err := os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !st.Mode().IsRegular() {
		return nil, false, fmt.Errorf("not a regular file: %s", rel)
	}
	if st.Size() > 8<<20 {
		return nil, false, fmt.Errorf("plugin file exceeds 8 MiB: %s", rel)
	}
	b, err := os.ReadFile(p)
	return b, true, err
}

func readState(root, host string) (state, error) {
	s := state{Version: 1, Host: host, Entries: map[string]entry{}}
	b, exists, err := readSafe(root, stateFile(host))
	if err != nil || !exists {
		return s, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&s); err != nil {
		return s, fmt.Errorf("invalid plugin state: %w", err)
	}
	if s.Version != 1 || s.Host != host {
		return s, errors.New("incompatible plugin state")
	}
	d, _ := Describe(host)
	for rel, e := range s.Entries {
		if _, err = safePath(root, rel); err != nil {
			return s, err
		}
		allowed := rel == d.ConfigPath || (d.HooksPath != "" && rel == d.HooksPath) || (d.AgentsPath != "" && strings.HasPrefix(rel, d.AgentsPath+"/")) || strings.HasPrefix(rel, d.SkillsPath+"/") || strings.HasPrefix(rel, "agentarch/project/plugins/"+host+"/") || strings.HasPrefix(rel, "agentarch/project/agents/agentarch-"+host+"/") || strings.HasPrefix(rel, "agentarch/project/tools/agentarch_"+host+"_")
		if !allowed || !contains([]string{"file", "json", "toml", "hooks"}, e.Kind) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(e.Hash) {
			return s, fmt.Errorf("invalid managed entry: %s", rel)
		}
	}
	return s, nil
}

func apply(root, host string, changes []mutation, r *Report) error {
	ignore, exists, err := readSafe(root, ".agentarch/plugin-state/.gitignore")
	if err != nil {
		return err
	}
	if exists && string(ignore) != "*\n!.gitignore\n" {
		return errors.New("plugin-state .gitignore changed; backups must remain excluded from version control")
	}
	before := map[string]snapshot{}
	for _, m := range changes {
		b, exists, err := readSafe(root, m.Path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if exists {
			st, _ := os.Stat(filepath.Join(root, m.Path))
			mode = st.Mode().Perm()
		}
		before[m.Path] = snapshot{b, exists, mode}
		action := "create"
		if exists {
			action = "update"
		}
		if m.Delete {
			action = "remove"
		}
		r.Changes = append(r.Changes, Change{m.Path, action})
		if exists && !m.Private {
			backup := ".agentarch/plugin-state/" + host + "/backups/" + digest([]byte(m.Path)) + "-" + digest(b) + ".bak"
			r.Backups = append(r.Backups, backup)
			if _, _, err := readSafe(root, backup); err != nil {
				return err
			}
		}
	}
	if r.DryRun || len(changes) == 0 {
		return nil
	}
	// Lock only the installer, not a host session. The dry run never creates this lock.
	lockRel := ".agentarch/plugin-state/install.lock"
	lockPath, err := safePath(root, lockRel)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("another plugin operation is active (or a stale install.lock needs inspection): %w", err)
	}
	lock.Close()
	defer os.Remove(lockPath)
	// Recheck every snapshot after acquiring the lock; do not act on stale observations.
	for rel, prev := range before {
		b, exists, err := readSafe(root, rel)
		if err != nil {
			return err
		}
		if exists != prev.exists || !bytes.Equal(b, prev.data) {
			return fmt.Errorf("file changed during preflight: %s", rel)
		}
	}
	// Exclude private backups before the first backup is written, including on failure.
	// Keeping this local ignore file after an interrupted operation is intentional.
	if err = atomicWrite(root, ".agentarch/plugin-state/.gitignore", []byte("*\n!.gitignore\n"), 0600); err != nil {
		return err
	}
	for _, m := range changes {
		prev := before[m.Path]
		if prev.exists && !m.Private {
			backup := ".agentarch/plugin-state/" + host + "/backups/" + digest([]byte(m.Path)) + "-" + digest(prev.data) + ".bak"
			if err = atomicWrite(root, backup, prev.data, 0600); err != nil {
				return err
			}
		}
	}
	var done []string
	for _, m := range changes {
		if m.Delete {
			p, _ := safePath(root, m.Path)
			err = os.Remove(p)
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		} else {
			mode := before[m.Path].mode
			if m.Private {
				mode = 0600
			}
			err = atomicWrite(root, m.Path, m.Data, mode)
		}
		if err != nil {
			var rollbackErr error
			for i := len(done) - 1; i >= 0; i-- {
				rel := done[i]
				prev := before[rel]
				if prev.exists {
					rollbackErr = errors.Join(rollbackErr, atomicWrite(root, rel, prev.data, prev.mode))
				} else {
					e := os.Remove(filepath.Join(root, rel))
					if !errors.Is(e, os.ErrNotExist) {
						rollbackErr = errors.Join(rollbackErr, e)
					}
				}
			}
			return errors.Join(fmt.Errorf("plugin write failed for %s: %w", m.Path, err), rollbackErr)
		}
		done = append(done, m.Path)
	}
	return nil
}

func atomicWrite(root, rel string, b []byte, mode os.FileMode) error {
	p, err := safePath(root, rel)
	if err != nil {
		return err
	}
	dirMode := os.FileMode(0755)
	if strings.HasPrefix(rel, ".agentarch/plugin-state/") {
		dirMode = 0700
	}
	if err = os.MkdirAll(filepath.Dir(p), dirMode); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".agentarch-write-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, p)
}

func parseJSON(b []byte) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(b)) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	// Duplicate keys are ambiguous for preservation and security; do not normalize them away.
	d := json.NewDecoder(bytes.NewReader(b))
	if err := uniqueJSON(d); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("trailing JSON content")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("config must be a JSON object")
	}
	return m, nil
}
func uniqueJSON(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return errors.New("duplicate or invalid JSON key")
			}
			seen[key] = true
			if err = uniqueJSON(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = uniqueJSON(d); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = d.Token()
	return err
}
func canonicalJSON(b []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
func jsonFragmentHash(b []byte) (string, error) {
	m, err := parseJSON(b)
	if err != nil {
		return "", err
	}
	servers, err := parseJSON(m["mcpServers"])
	if err != nil {
		return "", err
	}
	value, ok := servers["agentarch"]
	if !ok {
		return "", errors.New("agentarch MCP entry missing")
	}
	c, err := canonicalJSON(value)
	return digest(c), err
}

func mergeJSON(old []byte, desired map[string]any, prior entry, remove bool) ([]byte, entry, error) {
	m, err := parseJSON(old)
	if err != nil {
		return nil, entry{}, err
	}
	servers, err := parseJSON(m["mcpServers"])
	if err != nil {
		return nil, entry{}, fmt.Errorf("mcpServers: %w", err)
	}
	current, exists := servers["agentarch"]
	currentCanonical, _ := canonicalJSON(current)
	if prior.Hash != "" && (!exists || digest(currentCanonical) != prior.Hash) {
		return nil, entry{}, errors.New("managed MCP entry changed or missing")
	}
	e := entry{Kind: "json", Preserve: prior.Preserve}
	if remove {
		delete(servers, "agentarch")
	} else {
		b, _ := json.Marshal(desired)
		e.Hash = digest(b)
		if prior.Hash == "" && exists {
			if !bytes.Equal(currentCanonical, b) {
				return nil, e, errors.New("existing agentarch MCP entry is not owned by this installation")
			}
			e.Preserve = true
		}
		if bytes.Equal(currentCanonical, b) {
			return old, e, nil
		}
		servers["agentarch"] = b
	}
	if len(servers) == 0 {
		delete(m, "mcpServers")
	} else {
		m["mcpServers"], _ = json.Marshal(servers)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	return append(b, '\n'), e, err
}

const tomlBegin = "# BEGIN agentarch managed MCP\n"
const tomlEnd = "# END agentarch managed MCP\n"

func splitTOML(b []byte) (prefix, block, suffix string, err error) {
	s := string(b)
	a := strings.Index(s, tomlBegin)
	z := strings.Index(s, tomlEnd)
	if a < 0 && z < 0 {
		return s, "", "", nil
	}
	if a < 0 || z < a || strings.Count(s, tomlBegin) != 1 || strings.Count(s, tomlEnd) != 1 {
		return "", "", "", errors.New("invalid managed TOML markers")
	}
	end := z + len(tomlEnd)
	return s[:a], s[a:end], s[end:], nil
}
func tomlFragmentHash(b []byte) (string, error) {
	_, block, _, err := splitTOML(b)
	if err != nil {
		return "", err
	}
	if block == "" {
		return "", errors.New("agentarch MCP block missing")
	}
	return digest([]byte(block)), nil
}
func mergeTOML(old []byte, desired map[string]any, prior entry, remove bool) ([]byte, entry, error) {
	prefix, block, suffix, err := splitTOML(old)
	if err != nil {
		return nil, entry{}, err
	}
	if prior.Hash != "" && (block == "" || digest([]byte(block)) != prior.Hash) {
		return nil, entry{}, errors.New("managed MCP block changed or missing")
	}
	// Do not attempt to parse or rewrite arbitrary TOML. Reject ambiguous existing ownership.
	outside := prefix + suffix
	for _, line := range strings.Split(outside, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") && strings.Contains(line, "mcp_servers") && strings.Contains(line, "agentarch") {
			return nil, entry{}, errors.New("unmanaged agentarch MCP configuration already exists")
		}
	}
	if remove {
		if prior.AddedNewline && strings.HasSuffix(prefix, "\n") {
			prefix = strings.TrimSuffix(prefix, "\n")
		}
		return []byte(prefix + suffix), entry{}, nil
	}
	command, _ := json.Marshal(desired["command"])
	args, _ := json.Marshal(desired["args"])
	names, _ := json.Marshal(toolNames())
	newBlock := tomlBegin + "[mcp_servers.agentarch]\ncommand = " + string(command) + "\nargs = " + string(args) + "\nenabled_tools = " + string(names) + "\n" + tomlEnd
	e := entry{Kind: "toml", Hash: digest([]byte(newBlock)), Preserve: prior.Preserve, AddedNewline: prior.AddedNewline}
	if prior.Hash == "" && block != "" {
		if block != newBlock {
			return nil, e, errors.New("existing agentarch block is not owned by this installation")
		}
		e.Preserve = true
	}
	if block == "" && prefix != "" && !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
		e.AddedNewline = true
	}
	return []byte(prefix + newBlock + suffix), e, nil
}

func emptyConfig(b []byte, kind string) bool {
	if kind == "toml" {
		return len(bytes.TrimSpace(b)) == 0
	}
	m, err := parseJSON(b)
	return err == nil && len(m) == 0
}
