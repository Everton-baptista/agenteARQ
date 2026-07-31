module github.com/Everton-baptista/agenteARQ

// The floor is deliberately low, and it is load-bearing for the entry point.
//
// `go run github.com/Everton-baptista/agenteARQ/cmd/agentarch@latest` is the one command someone
// types to try this. A directive above the reader's Go does one of two things, and neither is a
// good first impression: with GOTOOLCHAIN=auto it silently downloads a whole toolchain before
// anything happens, and with GOTOOLCHAIN=local — normal in CI and slim images — it fails with a
// message about toolchain versions that has nothing to do with what was asked for.
//
// 1.22 is the real floor: `for range <int>`, and only in tests. Dependencies ask for less
// (jsonschema/v6 wants 1.21, x/text 1.18, yaml.v3 declares nothing). Raise this only when
// something genuinely needs it — headroom here is reach given away for free, and
// TestGoDirectiveStaysAtTheFloor fails when `go mod tidy` on a new machine raises it by itself.
go 1.22

require (
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
	golang.org/x/text v0.14.0
	gopkg.in/yaml.v3 v3.0.1
)
