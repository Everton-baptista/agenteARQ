package main

// `agentarch doctor` — what is missing on this machine and in this project, and how to fix it.
//
// The people AgenteARQ is for most often stop before the first check ever runs: Git is not
// initialised, there is no container runtime, the verifier image was never prepared, the
// assistant was never connected. Each of those used to surface as an error from whichever
// command needed it, in that command's vocabulary. doctor asks all of them at once, in the
// project's language, and for every gap says why it matters and the exact next step for this
// operating system.
//
// It is read-only. It changes nothing, installs nothing and contacts nothing beyond running the
// local `git`, container runtime and `agentarch` binaries to ask them questions.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/plugin"
	"github.com/Everton-baptista/agenteARQ/internal/workflow"
)

type doctorCheck struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Why    string `json:"why,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

type doctorReport struct {
	Root   string        `json:"root"`
	OS     string        `json:"os"`
	Ready  bool          `json:"ready"`
	Checks []doctorCheck `json:"checks"`
}

func cmdDoctor(args []string) int {
	f := flag.NewFlagSet("doctor", flag.ContinueOnError)
	root := f.String("root", ".", "project root")
	format := f.String("format", "text", "text or json")
	if err := f.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "doctor:", err)
		return exitUsage
	}
	lang = projectLanguage(abs)
	r := runDoctor(abs, runtime.GOOS, exec.LookPath, probeCommand)
	if *format == "json" {
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		_ = e.Encode(r)
	} else {
		printDoctor(r)
	}
	if !r.Ready {
		return exitUsage
	}
	return exitOK
}

type lookPathFunc func(string) (string, error)
type probeFunc func(dir, name string, args ...string) error

// probeCommand runs a short, bounded local command and reports only whether it succeeded.
func probeCommand(dir, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.Run()
}

func runDoctor(root, goos string, look lookPathFunc, probe probeFunc) doctorReport {
	r := doctorReport{Root: root, OS: goos, Ready: true}
	add := func(c doctorCheck) {
		if !c.OK {
			r.Ready = false
		}
		r.Checks = append(r.Checks, c)
	}

	// 1. Git: every piece of evidence is bound to a commit.
	if _, err := look("git"); err != nil {
		add(doctorCheck{ID: "git", Detail: dmsg("git_missing"), Why: dmsg("git_why"), Fix: installHint("git", goos)})
	} else if probe(root, "git", "rev-parse", "--show-toplevel") != nil {
		add(doctorCheck{ID: "git", Detail: dmsg("git_norepo"), Why: dmsg("git_why"), Fix: "git init && git add -A && git commit -m \"Start\""})
	} else {
		add(doctorCheck{ID: "git", OK: true, Detail: dmsg("git_ok")})
	}

	// 2. A container runtime: checks run in an isolated container or not at all.
	var engine string
	for _, name := range []string{"podman", "docker", "container"} {
		if _, err := look(name); err == nil {
			engine = name
			break
		}
	}
	switch {
	case engine == "":
		add(doctorCheck{ID: "container-runtime", Detail: dmsg("oci_missing"), Why: dmsg("oci_why"), Fix: installHint("oci", goos)})
	case engine == "container":
		// Apple's container CLI is detected so the owner hears about it, but verification does
		// not drive it yet: its isolation flags have not been verified against the contract.
		add(doctorCheck{ID: "container-runtime", Detail: dmsg("oci_apple"), Why: dmsg("oci_why"), Fix: installHint("oci", goos)})
	case engine == "docker" && probe(root, "docker", "info") != nil:
		add(doctorCheck{ID: "container-runtime", Detail: dmsg("oci_down"), Why: dmsg("oci_why"), Fix: dmsg("oci_start")})
	default:
		add(doctorCheck{ID: "container-runtime", OK: true, Detail: fmt.Sprintf(dmsg("oci_ok"), engine)})
	}

	// 3. The product workflow.
	initialized := false
	if _, err := os.Stat(filepath.Join(root, workflow.ProductPath)); err == nil {
		initialized = true
		add(doctorCheck{ID: "workflow", OK: true, Detail: dmsg("wf_ok")})
	} else {
		add(doctorCheck{ID: "workflow", Detail: dmsg("wf_missing"), Why: dmsg("wf_why"), Fix: "agentarch start"})
	}

	// 4. The verifier image for the chosen profile.
	if initialized {
		if s, err := workflow.New(root); err == nil {
			if rec, err := s.LoadVerifier(); err == nil && rec.Image != "" {
				add(doctorCheck{ID: "verifier-image", OK: true, Detail: fmt.Sprintf(dmsg("img_ok"), rec.Profile)})
			} else {
				add(doctorCheck{ID: "verifier-image", Detail: dmsg("img_missing"), Why: dmsg("img_why"), Fix: "agentarch workflow prepare-verifier"})
			}
		}
	}

	// 5. The binary the hooks and MCP server call must be on PATH.
	if _, err := look("agentarch"); err != nil {
		add(doctorCheck{ID: "agentarch-on-path", Detail: dmsg("bin_missing"), Why: dmsg("bin_why"), Fix: installHint("agentarch", goos)})
	} else {
		add(doctorCheck{ID: "agentarch-on-path", OK: true, Detail: dmsg("bin_ok")})
	}

	// 6. A connected assistant, and for Claude Code the real-time hooks.
	var connected []string
	for _, host := range plugin.Hosts() {
		if rep, err := plugin.Check(root, host); err == nil && rep.Installed {
			connected = append(connected, host)
			if len(rep.Issues) > 0 {
				add(doctorCheck{ID: "plugin-" + host, Detail: fmt.Sprintf(dmsg("plugin_drift"), host, strings.Join(rep.Issues, "; ")), Why: dmsg("plugin_why"), Fix: "agentarch plugin check " + host})
			}
		}
	}
	if len(connected) == 0 {
		add(doctorCheck{ID: "assistant", Detail: dmsg("plugin_missing"), Why: dmsg("plugin_why"), Fix: "agentarch plugin install claude --owner \"<your name>\" --contact \"<your email>\" --model-provider <provider> --model <exact model id>"})
	} else {
		add(doctorCheck{ID: "assistant", OK: true, Detail: fmt.Sprintf(dmsg("plugin_ok"), strings.Join(connected, ", "))})
	}
	return r
}

func printDoctor(r doctorReport) {
	fmt.Println(dmsg("title"))
	for _, c := range r.Checks {
		mark := "✓"
		if !c.OK {
			mark = "✗"
		}
		fmt.Printf("\n %s %s\n", mark, c.Detail)
		if c.Why != "" {
			fmt.Printf("   %s %s\n", dmsg("why"), c.Why)
		}
		if c.Fix != "" {
			fmt.Printf("   %s %s\n", dmsg("fix"), c.Fix)
		}
	}
	fmt.Println()
	if r.Ready {
		fmt.Println(dmsg("ready"))
	} else {
		fmt.Println(dmsg("not_ready"))
	}
}

// installHint names the usual way to install a prerequisite on this operating system. It is a
// pointer, not an installer: doctor never downloads or runs anything.
func installHint(what, goos string) string {
	hints := map[string]map[string]string{
		"git": {
			"darwin":  "xcode-select --install   (or: brew install git)",
			"windows": "winget install --id Git.Git -e",
			"linux":   "sudo apt install git   (Fedora: sudo dnf install git)",
		},
		"oci": {
			"darwin":  "brew install podman && podman machine init && podman machine start   (or install Docker Desktop)",
			"windows": "winget install RedHat.Podman   (Podman uses WSL2; or install Docker Desktop)",
			"linux":   "sudo apt install podman   (Fedora: sudo dnf install podman)",
		},
		"agentarch": {
			"darwin":  "curl -fsSL https://raw.githubusercontent.com/Everton-baptista/agenteARQ/main/scripts/install.sh | sh",
			"windows": "go install github.com/Everton-baptista/agenteARQ/cmd/agentarch@latest",
			"linux":   "curl -fsSL https://raw.githubusercontent.com/Everton-baptista/agenteARQ/main/scripts/install.sh | sh",
		},
	}
	if h, ok := hints[what][goos]; ok {
		return h
	}
	return hints[what]["linux"]
}

var doctorMessages = map[string]map[string]string{
	"en": {
		"title":          "AgenteARQ doctor — what this project needs",
		"why":            "Why:",
		"fix":            "Fix:",
		"ready":          "Everything is in place. Open the project in your assistant and ask it to continue.",
		"not_ready":      "Fix the items marked ✗, one at a time, then run agentarch doctor again.",
		"git_missing":    "Git is not installed.",
		"git_norepo":     "This folder is not a Git repository yet.",
		"git_ok":         "Git repository found.",
		"git_why":        "Every check and review is tied to an exact commit, so the work needs Git.",
		"oci_missing":    "No container runtime (Podman or Docker) is installed.",
		"oci_apple":      "Only Apple's container CLI was found; AgenteARQ does not drive it yet.",
		"oci_down":       "Docker is installed but not running.",
		"oci_start":      "Start Docker Desktop (or: sudo systemctl start docker), then run doctor again.",
		"oci_ok":         "Container runtime: %s.",
		"oci_why":        "Checks run in an isolated container with no network, so they cannot cheat or leak secrets. Without one, verification is inconclusive and the work cannot advance.",
		"wf_ok":          "Product workflow initialized.",
		"wf_missing":     "No product workflow in this folder yet.",
		"wf_why":         "The workflow records where the product stands, so neither you nor the assistant gets lost.",
		"img_ok":         "Verifier image prepared for profile %s.",
		"img_missing":    "The verifier image for this product has not been prepared.",
		"img_why":        "The checks need their tools inside a pinned image; it is built once on this machine.",
		"bin_missing":    "The agentarch command is not on your PATH.",
		"bin_ok":         "agentarch is on PATH.",
		"bin_why":        "Your assistant's hooks and the MCP server call agentarch by name.",
		"plugin_missing": "No coding assistant is connected to this project.",
		"plugin_ok":      "Connected assistants: %s.",
		"plugin_drift":   "The %s plugin files changed since install: %s.",
		"plugin_why":     "The plugin gives your assistant the skills, the workflow tools and (in Claude Code) the real-time guardrail hooks.",
	},
	"pt-BR": {
		"title":          "AgenteARQ doctor — o que este projeto precisa",
		"why":            "Por quê:",
		"fix":            "Como resolver:",
		"ready":          "Está tudo pronto. Abra o projeto no seu assistente e peça para continuar.",
		"not_ready":      "Resolva os itens marcados com ✗, um de cada vez, e rode agentarch doctor de novo.",
		"git_missing":    "O Git não está instalado.",
		"git_norepo":     "Esta pasta ainda não é um repositório Git.",
		"git_ok":         "Repositório Git encontrado.",
		"git_why":        "Cada verificação e revisão fica presa a um commit exato, por isso o trabalho precisa do Git.",
		"oci_missing":    "Nenhum runtime de contêiner (Podman ou Docker) está instalado.",
		"oci_apple":      "Só foi encontrado o container da Apple; o AgenteARQ ainda não o utiliza.",
		"oci_down":       "O Docker está instalado, mas não está rodando.",
		"oci_start":      "Abra o Docker Desktop (ou: sudo systemctl start docker) e rode o doctor de novo.",
		"oci_ok":         "Runtime de contêiner: %s.",
		"oci_why":        "As verificações rodam num contêiner isolado e sem rede, para não trapacear nem vazar segredos. Sem ele, a verificação fica inconclusiva e o trabalho não avança.",
		"wf_ok":          "Workflow do produto inicializado.",
		"wf_missing":     "Ainda não há workflow de produto nesta pasta.",
		"wf_why":         "O workflow registra onde o produto está, para que nem você nem o assistente se percam.",
		"img_ok":         "Imagem de verificação preparada para o perfil %s.",
		"img_missing":    "A imagem de verificação deste produto ainda não foi preparada.",
		"img_why":        "As verificações precisam das ferramentas dentro de uma imagem fixada; ela é construída uma vez nesta máquina.",
		"bin_missing":    "O comando agentarch não está no seu PATH.",
		"bin_ok":         "agentarch está no PATH.",
		"bin_why":        "Os hooks do seu assistente e o servidor MCP chamam o agentarch pelo nome.",
		"plugin_missing": "Nenhum assistente de código está conectado a este projeto.",
		"plugin_ok":      "Assistentes conectados: %s.",
		"plugin_drift":   "Os arquivos do plugin %s mudaram desde a instalação: %s.",
		"plugin_why":     "O plugin dá ao seu assistente as skills, as ferramentas do workflow e (no Claude Code) os hooks de proteção em tempo real.",
	},
}

func dmsg(key string) string {
	if m, ok := doctorMessages[lang][key]; ok {
		return m
	}
	return doctorMessages["en"][key]
}
