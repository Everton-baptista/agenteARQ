package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/mcp"
	"gopkg.in/yaml.v3"
)

func cmdMCP(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentarch mcp audit [--probe] [--record]")
		return exitUsage
	}
	switch args[0] {
	case "audit":
		return cmdMCPAudit(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown mcp subcommand %q\n", args[0])
		return exitUsage
	}
}

func cmdMCPAudit(args []string) int {
	fs_ := flag.NewFlagSet("mcp audit", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	probe := fs_.Bool("probe", false, "connect to each stdio server and compare live tool descriptions")
	record := fs_.Bool("record", false, "with --probe: write the current digests into the allowlist")
	maxAge := fs_.Int("max-review-age-days", 180, "report a server whose review is older than this")
	timeout := fs_.Duration("timeout", 15*time.Second, "per-server probe timeout")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() > 0 {
		*root = fs_.Arg(0)
	}

	a, path, err := mcp.Load(*root)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("no MCP allowlist at %s — nothing to audit\n", path)
			return exitOK
		}
		fmt.Fprintln(os.Stderr, "mcp audit:", err)
		return exitUsage
	}

	now := time.Now().UTC()
	findings := mcp.StaticAudit(a, now, *maxAge)

	if *probe {
		// Starting a process is the one thing agentarch otherwise never does. Auditing a
		// supply chain by running the supply chain has to be a decision, not a default.
		fmt.Fprintf(os.Stderr, "probing %d server(s) — this starts each stdio server locally\n", len(a.Servers))
		changed := false
		for i := range a.Servers {
			s := &a.Servers[i]
			if s.Transport != "stdio" {
				fmt.Printf("skip   %s (%s transport not probed)\n", s.Name, s.Transport)
				continue
			}
			live, err := mcp.Probe(*s, *timeout)
			if err != nil {
				findings = append(findings, mcp.Finding{
					Server: s.Name, ID: "MCP-PROBE",
					Message: "could not probe: " + err.Error(),
					Fix:     "a server that cannot be inspected cannot be reviewed. Check the command and pin.",
				})
				continue
			}
			fmt.Printf("probed %s — %d tool(s) offered\n", s.Name, len(live))

			if *record {
				if s.ToolDescriptionSHA == nil {
					s.ToolDescriptionSHA = map[string]string{}
				}
				for _, t := range live {
					for _, allowed := range s.ToolsAllow {
						if t.Name == allowed {
							d := mcp.DescriptionDigest(t.Description)
							if s.ToolDescriptionSHA[t.Name] != d {
								s.ToolDescriptionSHA[t.Name] = d
								changed = true
							}
						}
					}
				}
				s.ReviewedAt = now.Format("2006-01-02")
				continue
			}
			findings = append(findings, mcp.CompareLive(*s, live)...)
		}

		if *record && changed {
			buf, err := yaml.Marshal(a)
			if err != nil {
				fmt.Fprintln(os.Stderr, "mcp audit:", err)
				return exitUsage
			}
			header := "# MCP allowlist. Generated digests record what each tool description said\n" +
				"# when it was reviewed — read them before committing. `agentarch sync` derives\n" +
				"# .mcp.json from this file, so this is the source and that is the derivative.\n"
			if err := os.WriteFile(path, append([]byte(header), buf...), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, "mcp audit:", err)
				return exitUsage
			}
			fmt.Printf("\nrecorded current digests in %s\n", path)
			fmt.Println("Read the descriptions before committing this: --record captures what the")
			fmt.Println("server says today, which is only meaningful if a person approved it.")
			return exitOK
		}
	}

	// Compare the generated .mcp.json with what the allowlist would produce, so a
	// hand-edited runtime config does not quietly diverge from the reviewed document.
	if want, err := mcp.RenderMCPJSON(a); err == nil {
		mcpPath := filepath.Join(*root, ".mcp.json")
		if got, err := os.ReadFile(mcpPath); err == nil {
			if string(got) != string(want)+"\n" && string(got) != string(want) {
				findings = append(findings, mcp.Finding{
					ID:      "MCP-DRIFT",
					Message: ".mcp.json does not match what the allowlist would generate",
					Fix:     "run `agentarch sync --targets mcp_json`. The allowlist is the source; .mcp.json is derived from it.",
				})
			}
		}
	}

	if len(findings) == 0 {
		fmt.Printf("mcp audit: %d server(s), no findings\n", len(a.Servers))
		return exitOK
	}

	critical := 0
	for _, f := range findings {
		fmt.Fprintln(os.Stderr, f)
		if f.Critical {
			critical++
		}
	}
	fmt.Fprintf(os.Stderr, "\n%d finding(s), %d critical\n", len(findings), critical)
	if critical > 0 {
		return exitGate
	}
	return exitStructure
}
