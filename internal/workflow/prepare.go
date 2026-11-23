package workflow

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Verification only runs in a prepared image, and asking every person to choose a base, find
// its digest, clone this repository and run a build script meant nobody got past `verify`.
// PrepareVerifier builds that image locally from the recipe shipped inside the binary, with a
// reviewed default base per profile, and records it so `verify` uses it.
//
// Building is the one network-enabled step and it is explicit: verify itself never builds,
// pulls or falls back to the host.

// DefaultVerifierBases are reviewed, digest-pinned bases. A profile missing here needs --base,
// because its right base depends on the project (a Node or a Playwright image, for instance).
var DefaultVerifierBases = map[string]string{
	"python-api":  "docker.io/library/python:3.12-slim@sha256:5e2bdb1b0d542d5039cffb8181c65930093949194a65c365dfb1b0fe13a4fa5b",
	"go-service":  "docker.io/library/golang:1.27-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea",
	"static-site": "docker.io/library/node:22.23.4-bookworm-slim@sha256:5ec23b7c622b00547753bc6ffd78085dd868a68aeb41c8e0a1418f033e13d96d",
	"docs":        "docker.io/library/node:22.23.4-bookworm-slim@sha256:5ec23b7c622b00547753bc6ffd78085dd868a68aeb41c8e0a1418f033e13d96d",
}

// baseAdvice explains the choice when a profile has no default, instead of picking one.
var baseAdvice = map[string]string{
	"typescript-web": "Pass --base with a Node image if your tests need no browser, or a Playwright image whose version matches the @playwright/test in your package-lock.json if they do. Pin it by digest: docker pull <image:tag> && docker image inspect --format '{{index .RepoDigests 0}}' <image:tag>",
}

// verifierInputHelp says how to produce a dependency input the project lacks.
var verifierInputHelp = map[string]string{
	"requirements-verifier.lock": "a hash-pinned lock of your runtime dependencies plus ruff, mypy, pytest and bandit, e.g. `pip-compile --generate-hashes -o requirements-verifier.lock requirements.in`",
	"package-lock.json":          "run `npm install` once and commit package-lock.json",
	"go.sum":                     "run `go mod tidy`",
}

var pinnedImage = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}$`)

type PrepareOptions struct {
	Recipes fs.FS     // the embedded tree whose root contains verifiers/
	Base    string    // optional override, digest-pinned
	Engine  string    // docker or podman; empty picks podman, then docker
	Out     io.Writer // build progress
}

// VerifierRecord is the image prepared for this worktree. It is local state, never committed.
type VerifierRecord struct {
	Profile string    `json:"profile"`
	Image   string    `json:"image"`
	Base    string    `json:"base"`
	BuiltAt time.Time `json:"built_at"`
}

func (s *Service) verifierPath() string { return filepath.Join(s.localDir(), "verifier.json") }

func (s *Service) LoadVerifier() (*VerifierRecord, error) {
	b, e := os.ReadFile(s.verifierPath())
	if e != nil {
		return nil, e
	}
	var r VerifierRecord
	if e = json.Unmarshal(b, &r); e != nil {
		return nil, e
	}
	return &r, nil
}

func pickEngine(engine string) (string, error) {
	if engine != "" {
		if engine != "docker" && engine != "podman" {
			return "", errors.New("engine must be docker or podman")
		}
		return exec.LookPath(engine)
	}
	for _, name := range []string{"podman", "docker"} {
		if p, e := exec.LookPath(name); e == nil {
			return p, nil
		}
	}
	return "", errors.New("install Podman or Docker: verification runs in a container")
}

// copySources resolves each COPY source of the recipe: the recipe's own directory, a shared
// directory of verifiers/, or a dependency manifest from the project.
func (s *Service) stageRecipe(recipes fs.FS, profile, dir string) error {
	recipeDir := path.Join("verifiers", profile)
	dockerfile, e := fs.ReadFile(recipes, path.Join(recipeDir, "Dockerfile"))
	if e != nil {
		return fmt.Errorf("this build ships no verifier recipe for profile %q", profile)
	}
	if e = os.WriteFile(filepath.Join(dir, "Dockerfile"), dockerfile, 0o644); e != nil {
		return e
	}
	sc := bufio.NewScanner(strings.NewReader(string(dockerfile)))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 || !strings.EqualFold(fields[0], "COPY") {
			continue
		}
		var srcs []string
		for _, f := range fields[1 : len(fields)-1] {
			if !strings.HasPrefix(f, "--") {
				srcs = append(srcs, strings.TrimSuffix(f, "/"))
			}
		}
		for _, src := range srcs {
			if strings.Contains(src, "..") || strings.HasPrefix(src, "/") {
				return fmt.Errorf("recipe COPY source %q is not a plain relative path", src)
			}
			switch {
			case src == "floor":
				e = copyTree(recipes, "verifiers/floor", filepath.Join(dir, "floor"))
			case exists(recipes, path.Join(recipeDir, src)):
				e = copyTree(recipes, path.Join(recipeDir, src), filepath.Join(dir, src))
			case exists(recipes, path.Join("verifiers", src)):
				e = copyTree(recipes, path.Join("verifiers", src), filepath.Join(dir, src))
			default:
				e = s.copyProjectInput(src, dir)
			}
			if e != nil {
				return e
			}
		}
	}
	return sc.Err()
}

func exists(fsys fs.FS, name string) bool {
	_, e := fs.Stat(fsys, name)
	return e == nil
}

func copyTree(fsys fs.FS, from, to string) error {
	return fs.WalkDir(fsys, from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, from), "/")
		target := filepath.Join(to, filepath.FromSlash(rel))
		if d.IsDir() {
			if d.Name() == "fixtures" || d.Name() == "tests" {
				return fs.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasPrefix(path.Base(p), "agentarch-") || strings.HasSuffix(p, ".sh") {
			mode = 0o755
		}
		if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, b, mode)
	})
}

// copyProjectInput copies one dependency manifest from the project. Only regular files are
// accepted, so a symlink cannot pull a host file into the build context.
func (s *Service) copyProjectInput(rel, dir string) error {
	src, e := safePath(s.Root, rel)
	if e != nil {
		return e
	}
	info, e := os.Lstat(src)
	if e != nil || !info.Mode().IsRegular() {
		if rel == "go.sum" { // a module without dependencies has none
			return os.WriteFile(filepath.Join(dir, rel), nil, 0o644)
		}
		msg := fmt.Sprintf("the profile's recipe needs %s from the project", rel)
		if help, ok := verifierInputHelp[rel]; ok {
			msg += ": " + help
		}
		return errors.New(msg)
	}
	b, e := os.ReadFile(src)
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(dir, filepath.Base(rel)), b, 0o644)
}

// PrepareVerifier builds the verifier image for the product's profile and records it.
func (s *Service) PrepareVerifier(ctx context.Context, o PrepareOptions) (VerifierRecord, error) {
	p, _, e := s.loadArtifacts()
	if e != nil {
		return VerifierRecord{}, e
	}
	base := strings.TrimSpace(o.Base)
	if base == "" {
		base = DefaultVerifierBases[p.Profile]
	}
	if base == "" && p.Profile == "universal" {
		return VerifierRecord{}, fmt.Errorf("the universal profile runs the commands declared in product.yaml, so its image must contain their toolchain; AgenteARQ does not guess it. %s", UniversalBaseOptions(p.Checks))
	}
	if base == "" {
		advice := baseAdvice[p.Profile]
		if advice == "" {
			advice = "Pass --base with a reviewed image pinned by @sha256 digest that provides this profile's tools."
		}
		return VerifierRecord{}, fmt.Errorf("profile %s has no default base image. %s", p.Profile, advice)
	}
	if !pinnedImage.MatchString(base) {
		return VerifierRecord{}, errors.New("--base must be an image reference pinned by @sha256 digest")
	}
	engine, e := pickEngine(o.Engine)
	if e != nil {
		return VerifierRecord{}, e
	}
	dir, e := os.MkdirTemp("", "agentarch-verifier-*")
	if e != nil {
		return VerifierRecord{}, e
	}
	defer os.RemoveAll(dir)
	if e = s.stageRecipe(o.Recipes, p.Profile, dir); e != nil {
		return VerifierRecord{}, e
	}
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	tag := "localhost/agentarch-verifier-" + p.Profile + ":latest"
	fmt.Fprintf(out, "building the %s verifier from %s (network enabled for this build only; no source or credentials are copied)\n", p.Profile, base)
	args := []string{"build", "--network=default", "--build-arg", "BASE_IMAGE=" + base}
	if p.BrowserChecks && p.Profile == "static-site" {
		args = append(args, "--build-arg", "WITH_BROWSER=1")
	}
	args = append(args, "--tag", tag, "--file", filepath.Join(dir, "Dockerfile"), dir)
	build := exec.CommandContext(ctx, engine, args...)
	build.Stdout, build.Stderr = out, out
	if e = build.Run(); e != nil {
		return VerifierRecord{}, fmt.Errorf("verifier build failed: %w", e)
	}
	raw, e := exec.CommandContext(ctx, engine, "image", "inspect", "--format", "{{.Id}}", tag).Output()
	if e != nil {
		return VerifierRecord{}, fmt.Errorf("cannot read the built image id: %w", e)
	}
	id := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(id, "sha256:") {
		id = "sha256:" + id
	}
	if !localImageID.MatchString(id) {
		return VerifierRecord{}, fmt.Errorf("unexpected image id %q", id)
	}
	r := VerifierRecord{Profile: p.Profile, Image: id, Base: base, BuiltAt: now()}
	b, _ := json.MarshalIndent(r, "", "  ")
	if e = os.MkdirAll(s.localDir(), 0o700); e != nil {
		return VerifierRecord{}, e
	}
	if e = atomicWrite(s.verifierPath(), append(b, '\n'), 0o600); e != nil {
		return VerifierRecord{}, e
	}
	return r, nil
}

// localImageID is a content-addressed local image: as fixed as a registry digest on the machine
// that built it, and the only identity a local build has.
var localImageID = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// toolchainImages maps a declared check's program to the official images that provide it. They
// are options to compare, not defaults: the owner picks one and pins it by digest.
var toolchainImages = []struct {
	programs []string
	image    string
}{
	{[]string{"python", "python3", "pytest", "ruff", "mypy", "pip", "uv", "poetry"}, "docker.io/library/python:<version>-slim"},
	{[]string{"node", "npm", "npx", "pnpm", "yarn"}, "docker.io/library/node:<version>-bookworm-slim"},
	{[]string{"go", "gofmt"}, "docker.io/library/golang:<version>-bookworm"},
	{[]string{"cargo", "rustc", "rustfmt"}, "docker.io/library/rust:<version>-slim"},
	{[]string{"java", "mvn", "gradle", "./gradlew", "./mvnw"}, "docker.io/library/eclipse-temurin:<version>-jdk (add Maven or Gradle)"},
	{[]string{"dotnet"}, "mcr.microsoft.com/dotnet/sdk:<version>"},
	{[]string{"php", "composer", "phpunit"}, "docker.io/library/php:<version>-cli (add Composer)"},
	{[]string{"ruby", "bundle", "rake", "rspec"}, "docker.io/library/ruby:<version>-slim"},
	{[]string{"flutter", "dart"}, "docker.io/library/dart:<version> (Flutter needs its own SDK image)"},
	{[]string{"swift"}, "docker.io/library/swift:<version>"},
	{[]string{"elixir", "mix"}, "docker.io/library/elixir:<version>-slim"},
}

// UniversalBaseOptions names candidate images for the toolchains the declared checks call, and
// how to pin the chosen one. With no recognisable program it says what to look for instead.
func UniversalBaseOptions(checks []CheckSpec) string {
	seen := map[string]bool{}
	var options []string
	for _, c := range checks {
		prog := c.Program
		if i := strings.LastIndex(prog, "/"); i >= 0 && !strings.HasPrefix(prog, "./") {
			prog = prog[i+1:]
		}
		for _, t := range toolchainImages {
			for _, p := range t.programs {
				if p == prog && !seen[t.image] {
					seen[t.image] = true
					options = append(options, t.image)
				}
			}
		}
	}
	how := "Pin the one you choose: docker pull <image:tag> && docker image inspect --format '{{index .RepoDigests 0}}' <image:tag>, then pass it with --base and record the choice in an ADR."
	if len(options) == 0 {
		return "Choose an official image that provides every program your checks run. " + how
	}
	return "Images that provide the programs your checks run: " + strings.Join(options, "; ") + ". " + how
}
