package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// This capability is supplied by the loader, never deserialized from a manifest.
// The expression evaluator only produces a string; filesystem access belongs to
// the file_exists gate and remains confined to the loaded project.
const fileContextKey = "_agentarch_file_context"

type fileContext struct{ root, agentDir string }

func addFileContext(ctx map[string]any, root, agentDir string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absAgent, err := filepath.Abs(agentDir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absAgent)
	if err != nil || outsideProject(rel) {
		return fmt.Errorf("agent directory is outside project root")
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return fmt.Errorf("resolve file check root: %w", err)
	}
	ctx[fileContextKey] = fileContext{root: realRoot, agentDir: filepath.Join(realRoot, rel)}
	return nil
}

// Layout paths are project-relative; expose a derived agent-relative path so
// file_exists retains its specified base directory for every control.
func addContractPath(ctx map[string]any, root, agentDir string) {
	config, _ := ctx["config"].(map[string]any)
	layout, _ := config["layout"].(map[string]any)
	paths, _ := layout["paths"].(map[string]any)
	contract, _ := paths["contract"].(string)
	if contract == "" || filepath.IsAbs(contract) || filepath.VolumeName(contract) != "" {
		return
	}
	clean := filepath.Clean(contract)
	if outsideProject(clean) {
		return
	}
	rel, err := filepath.Rel(agentDir, filepath.Join(root, clean))
	if err != nil {
		return
	}
	repo, _ := ctx["repository"].(map[string]any)
	if repo != nil {
		repo["contract_path"] = filepath.ToSlash(rel)
	}
}

func checkArtifactFile(expr string, ctx map[string]any, now time.Time) (bool, error) {
	fc, ok := ctx[fileContextKey].(fileContext)
	if !ok {
		return false, fmt.Errorf("file_exists requires a project file context from LoadAgents")
	}
	if strings.TrimSpace(expr) == "" {
		return false, fmt.Errorf("file_exists requires path_expr")
	}
	n, err := Parse(expr)
	if err != nil {
		return false, fmt.Errorf("file_exists path_expr: %w", err)
	}
	v, err := (&evaluator{ctx: ctx, now: now}).eval(n)
	if err != nil {
		return false, fmt.Errorf("file_exists path_expr: %w", err)
	}
	if v == nil {
		return false, nil
	}
	path, ok := v.(string)
	if !ok {
		return false, fmt.Errorf("file_exists path_expr must yield one path string")
	}
	if strings.TrimSpace(path) == "" {
		return false, nil
	}
	if filepath.IsAbs(path) || filepath.VolumeName(path) != "" || strings.ContainsRune(path, '\x00') || strings.Contains(path, "\\") {
		return false, fmt.Errorf("file_exists path must be a relative project path")
	}
	full := filepath.Join(fc.agentDir, filepath.FromSlash(path))
	rel, err := filepath.Rel(fc.root, full)
	if err != nil || outsideProject(rel) {
		return false, fmt.Errorf("file_exists path escapes the project root")
	}
	// Reject all symlink components, including internal aliases: the gate should
	// verify the declared artifact rather than follow a replaceable redirect.
	current := fc.root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("file_exists cannot inspect artifact: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("file_exists refuses symlinked artifacts")
		}
		if i == len(parts)-1 {
			return info.Mode().IsRegular(), nil
		}
		if !info.IsDir() {
			return false, nil
		}
	}
	return false, nil
}

func outsideProject(path string) bool {
	return path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) || filepath.IsAbs(path)
}
