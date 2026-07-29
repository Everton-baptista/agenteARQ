// Package agentarch carries the standard's payload — the content and spec trees — embedded
// into the binary.
//
// The embed directives live at the module root because go:embed cannot reach outside its own
// package directory, and the payload must stay where humans edit it rather than being copied
// into the CLI's source tree.
//
// Embedding is what lets `agentarch init` work offline and without network access. That is a
// deliberate constraint: a governance tool that phones home to fetch the rules it enforces is
// one outage away from being unusable, and one compromise away from being dangerous.
package agentarch

import "embed"

// all: is required. Without it go:embed silently skips anything beginning with a dot, and the
// blueprints ship a .github/workflows — so the CI gate never reached the project and a fresh
// install could not reach conformance L2. Silently, because an absent file looks like a choice.
//
//go:embed all:content
var Content embed.FS

//go:embed spec/schemas spec/normative spec/conformance spec/VERSION
var Spec embed.FS
