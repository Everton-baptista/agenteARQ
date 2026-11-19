package policy

// Exit codes, and the one rule that orders them.
//
// spec/normative/06-exit-codes.md is normative for both. They live here, in one named place,
// rather than as an if-chain inside each command: the precedence is a MUST that a second
// implementation has to reproduce, and a rule spread across call sites is one that drifts
// between commands without anybody deciding it should.

// Exit codes. An implementation MUST use these and MUST NOT reuse them for other conditions —
// continuous integration routes on them, and a tool that returns 1 for everything is a tool
// whose failures get a blanket `|| true`.
const (
	ExitOK          = 0 // success
	ExitUsage       = 1 // usage error, internal error, or version incompatibility
	ExitStructure   = 2 // structural validation failed
	ExitDrift       = 3 // generated files are out of date
	ExitGate        = 4 // a blocker-severity control failed
	ExitWaiver      = 5 // a waiver is invalid or has expired
	ExitRevalidated = 6 // a revalidation trigger fired without revalidation
)

// Condition is one thing that can be true when a command finishes. More than one may hold.
type Condition string

const (
	CondUsage        Condition = "usage"
	CondStructure    Condition = "structure"
	CondDrift        Condition = "drift"
	CondWaiver       Condition = "waiver"
	CondBlocker      Condition = "blocker"
	CondRevalidation Condition = "revalidation"
)

// exitPrecedence is the order 06 declares: 1, 2, 3, 5, 4, 6.
//
// Waiver problems (5) precede a blocked gate (4) deliberately. A lapsed waiver usually explains
// the blocker, and reporting the blocker first sends the reader to fix something that is already
// tracked by somebody who agreed to a date.
var exitPrecedence = []struct {
	cond Condition
	code int
}{
	{CondUsage, ExitUsage},
	{CondStructure, ExitStructure},
	{CondDrift, ExitDrift},
	{CondWaiver, ExitWaiver},
	{CondBlocker, ExitGate},
	{CondRevalidation, ExitRevalidated},
}

// ExitCode returns the code to report when the given conditions hold, applying the precedence
// in 06. No conditions is success.
func ExitCode(conds ...Condition) int {
	held := make(map[Condition]bool, len(conds))
	for _, c := range conds {
		held[c] = true
	}
	for _, p := range exitPrecedence {
		if held[p.cond] {
			return p.code
		}
	}
	return ExitOK
}
