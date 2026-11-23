package plugin

// Real-time guardrail hooks for hosts that run them.
//
// Skills and the MCP server guide an assistant; a host hook is the one place where AgenteARQ can
// decide before a tool call happens. Claude Code runs project hooks from .claude/settings.json,
// so that is where the install writes them, as an owned fragment merged into whatever the file
// already holds: the user's own hooks for the same events, their permissions and every other
// setting survive install and removal, and a changed agentarch fragment is a conflict rather
// than something silently overwritten — the same contract the MCP entry follows.
//
// Each hook only calls `agentarch hook --event <e> --host claude`; every decision lives in that
// command, where it is tested, not in configuration.

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// hookEventsByHost maps AgenteARQ's host-neutral events onto a host's event names, with the
// tool matcher a pre-tool hook needs. Only hosts whose hook protocol `agentarch hook` speaks
// appear here.
var hookEventsByHost = map[string][]struct {
	Event, HostEvent, Matcher string
	Timeout                   int
}{
	"claude": {
		{"session-start", "SessionStart", "", 15},
		{"pre-tool", "PreToolUse", "Edit|Write|MultiEdit|NotebookEdit|Bash", 10},
		{"stop", "Stop", "", 30},
	},
}

func hookCommand(binary, host, event string) string {
	return shellQuote(binary) + " hook --event " + event + " --host " + host
}

// shellQuote quotes a binary path only when it needs it, so the common `agentarch` stays
// readable in the settings file.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '_' || r == '-' || r == ':' || r == '\\' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// hookFragment is the desired agentarch hook groups, keyed by the host's event name.
func hookFragment(host string, o Options) map[string][]any {
	out := map[string][]any{}
	for _, e := range hookEventsByHost[host] {
		group := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hookCommand(o.Binary, host, e.Event), "timeout": e.Timeout}}}
		if e.Matcher != "" {
			group["matcher"] = e.Matcher
		}
		out[e.HostEvent] = append(out[e.HostEvent], group)
	}
	return out
}

// ownedGroup reports whether a hook group was written by an agentarch install: every command
// in it is an `agentarch hook` invocation.
func ownedGroup(raw json.RawMessage) bool {
	var g struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if json.Unmarshal(raw, &g) != nil || len(g.Hooks) == 0 {
		return false
	}
	for _, h := range g.Hooks {
		if !strings.Contains(h.Command, " hook --event ") || !strings.Contains(h.Command, " --host ") {
			return false
		}
	}
	return true
}

// splitHooks separates the owned groups from everything else in a settings file's "hooks".
func splitHooks(settings map[string]json.RawMessage) (owned map[string][]json.RawMessage, others map[string][]json.RawMessage, err error) {
	owned, others = map[string][]json.RawMessage{}, map[string][]json.RawMessage{}
	hooks, err := parseJSON(settings["hooks"])
	if err != nil {
		return nil, nil, err
	}
	for event, raw := range hooks {
		var groups []json.RawMessage
		if err := json.Unmarshal(raw, &groups); err != nil {
			return nil, nil, errors.New("hooks." + event + " must be an array")
		}
		for _, g := range groups {
			if ownedGroup(g) {
				owned[event] = append(owned[event], g)
			} else {
				others[event] = append(others[event], g)
			}
		}
	}
	return owned, others, nil
}

func fragmentDigest(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	c, err := canonicalJSON(b)
	return digest(c), err
}

func hooksFragmentHash(b []byte) (string, error) {
	m, err := parseJSON(b)
	if err != nil {
		return "", err
	}
	owned, _, err := splitHooks(m)
	if err != nil {
		return "", err
	}
	return fragmentDigest(owned)
}

// mergeHooks writes (or, with remove, deletes) the owned hook groups in a settings file.
func mergeHooks(old []byte, desired map[string][]any, prior entry, remove bool) ([]byte, entry, error) {
	m, err := parseJSON(old)
	if err != nil {
		return nil, entry{}, err
	}
	owned, others, err := splitHooks(m)
	if err != nil {
		return nil, entry{}, err
	}
	current, err := fragmentDigest(owned)
	if err != nil {
		return nil, entry{}, err
	}
	if prior.Hash != "" && current != prior.Hash {
		return nil, entry{}, errors.New("managed agentarch hooks changed; restore them or remove them by hand before installing")
	}
	e := entry{Kind: "hooks", Preserve: prior.Preserve}
	if !remove {
		e.Hash, err = fragmentDigest(desired)
		if err != nil {
			return nil, e, err
		}
		if prior.Hash == "" && len(owned) > 0 {
			if current != e.Hash {
				return nil, e, errors.New("existing agentarch hooks are not owned by this installation")
			}
			e.Preserve = true
		}
		if current == e.Hash {
			return old, e, nil
		}
	}
	events := map[string]bool{}
	for k := range others {
		events[k] = true
	}
	if !remove {
		for k := range desired {
			events[k] = true
		}
	}
	names := make([]string, 0, len(events))
	for k := range events {
		names = append(names, k)
	}
	sort.Strings(names)
	hooks := map[string]json.RawMessage{}
	for _, event := range names {
		groups := append([]any{}, toAny(others[event])...)
		if !remove {
			groups = append(groups, desired[event]...)
		}
		if len(groups) == 0 {
			continue
		}
		hooks[event], _ = json.Marshal(groups)
	}
	if len(hooks) == 0 {
		delete(m, "hooks")
	} else {
		m["hooks"], _ = json.Marshal(hooks)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, e, err
	}
	b = append(b, '\n')
	if bytes.Equal(b, old) {
		return old, e, nil
	}
	return b, e, nil
}

func toAny(raws []json.RawMessage) []any {
	out := make([]any, len(raws))
	for i, r := range raws {
		out[i] = r
	}
	return out
}
