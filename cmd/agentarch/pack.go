package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/Everton-baptista/agenteARQ/internal/policy"
	"github.com/Everton-baptista/agenteARQ/internal/registry"
)

func cmdPack(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentarch pack list|verify|add [<id>]")
		return exitUsage
	}
	switch args[0] {
	case "list":
		return cmdPackList(args[1:])
	case "verify":
		return cmdPackVerify(args[1:])
	case "add":
		return cmdPackAdd(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown pack subcommand %q\n", args[0])
		return exitUsage
	}
}

// registryPath prefers a project's own index, falling back to the one shipped with the CLI.
func registryPath(root string) string {
	local := filepath.Join(root, "registry", "index.yaml")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return ""
}

func cmdPackList(args []string) int {
	fs_ := flag.NewFlagSet("pack list", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	installed := fs_.Bool("installed", false, "list the packs installed in this project instead")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}

	if *installed {
		cfs, err := contentFS(*root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pack list:", err)
			return exitUsage
		}
		cat, err := policy.LoadCatalog(cfs)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pack list:", err)
			return exitUsage
		}
		var ids []string
		for id := range cat.Packs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		fmt.Printf("\ninstalled packs (%d)\n\n", len(ids))
		for _, id := range ids {
			p := cat.Packs[id]
			fmt.Printf("  %-18s %-8s %-22s %d control(s), reviewed %s\n",
				p.ID, p.Version, p.AuthorityStatus, len(p.Requires), p.ReviewedAt)
		}
		fmt.Println()
		return exitOK
	}

	path := registryPath(*root)
	if path == "" {
		fmt.Println("no registry index found. The official packs are installed with the standard;")
		fmt.Println("run `agentarch pack list --installed` to see them.")
		return exitOK
	}
	idx, err := registry.LoadIndex(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pack list:", err)
		return exitUsage
	}
	if len(idx.Entries) == 0 {
		fmt.Printf("%s lists no entries yet.\n", path)
		return exitOK
	}
	fmt.Printf("\ncommunity registry — %d entr(y/ies)\n\n", len(idx.Entries))
	for _, e := range idx.Entries {
		fmt.Printf("  %-22s %-8s %-16s %s\n", e.ID, e.Version, e.Trust, e.Title)
		fmt.Printf("  %-22s owner %s · %s · reviewed %s\n", "", e.Owner, e.Licence, e.ReviewedAt)
	}
	fmt.Println("\n  Listing is not endorsement. `trust` says what is actually known.")
	fmt.Println()
	return exitOK
}

func cmdPackVerify(args []string) int {
	fs_ := flag.NewFlagSet("pack verify", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}

	path := registryPath(*root)
	if path == "" {
		fmt.Println("no registry index to verify")
		return exitOK
	}
	idx, err := registry.LoadIndex(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pack verify:", err)
		return exitUsage
	}
	problems := idx.Problems(time.Now().UTC())
	if len(problems) == 0 {
		fmt.Printf("pack verify: %d entr(y/ies), no problems\n", len(idx.Entries))
		return exitOK
	}
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "  "+p)
	}
	fmt.Fprintf(os.Stderr, "\n%d problem(s). A registry listing things nobody can verify teaches\n"+
		"people that the checksums are decorative.\n", len(problems))
	return exitStructure
}

func cmdPackAdd(args []string) int {
	fs_ := flag.NewFlagSet("pack add", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	timeout := fs_.Duration("timeout", 30*time.Second, "download timeout")
	yes := fs_.Bool("yes", false, "skip the confirmation prompt")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: agentarch pack add <id>")
		return exitUsage
	}
	id := fs_.Arg(0)

	path := registryPath(*root)
	if path == "" {
		fmt.Fprintln(os.Stderr, "pack add: no registry index found")
		return exitUsage
	}
	idx, err := registry.LoadIndex(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pack add:", err)
		return exitUsage
	}
	e, ok := idx.Find(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "pack add: %q is not in %s\n", id, path)
		return exitUsage
	}

	// Adding a pack changes which rules a project is judged by. Saying who published it and
	// what is known about them, before fetching, is the point at which a person can decide.
	fmt.Printf("\n%s  %s\n", e.ID, e.Version)
	fmt.Printf("  %s\n", e.Title)
	fmt.Printf("  owner    %s\n", e.Owner)
	fmt.Printf("  licence  %s\n", e.Licence)
	fmt.Printf("  trust    %s — %s\n", e.Trust, idx.TrustLevels[e.Trust])
	fmt.Printf("  source   %s\n", e.URL)
	fmt.Printf("  sha256   %s\n\n", e.SHA256)
	fmt.Printf("This adds rules your gate will enforce. The checksum is verified before anything\n")
	fmt.Printf("is written, and no content from a pack is ever executed.\n\n")

	if !*yes {
		fmt.Fprintln(os.Stderr, "Re-run with --yes to install.")
		return exitUsage
	}

	body, err := registry.Fetch(e, nil, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pack add:", err)
		return exitUsage
	}

	dest := filepath.Join(*root, "agentarch", "std", "packs", e.ID)
	written, err := registry.Unpack(body, dest)
	if err != nil {
		// Leave nothing half-written behind when an archive turns out to be hostile.
		_ = os.RemoveAll(dest)
		fmt.Fprintln(os.Stderr, "pack add:", err)
		return exitUsage
	}

	fmt.Printf("installed %s into %s (%d file(s))\n", e.ID, dest, len(written))
	fmt.Printf("Add it to a profile or to an agent's policy.packs, then run `agentarch check`.\n")
	return exitOK
}

// shippedRegistry is here so the embedded payload is reachable if the CLI ever ships one.
var _ fs.FS = agentarch.Content
