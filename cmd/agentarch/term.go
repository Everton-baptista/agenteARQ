package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// colorEnabled reports whether ANSI color codes should be emitted.
// Respects NO_COLOR standard (https://no-color.org) and terminal capability.
func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return isTTY()
}

// ANSI Escape Codes
const (
	ansiReset       = "\033[0m"
	ansiBold        = "\033[1m"
	ansiDim         = "\033[2m"
	ansiItalic      = "\033[3m"
	ansiUnderline   = "\033[4m"
	ansiRed         = "\033[31m"
	ansiGreen       = "\033[32m"
	ansiYellow      = "\033[33m"
	ansiBlue        = "\033[34m"
	ansiMagenta     = "\033[35m"
	ansiCyan        = "\033[36m"
	ansiBrightCyan  = "\033[96m"
	ansiBrightBlue  = "\033[94m"
	ansiGray        = "\033[90m"
	ansiBgGreen     = "\033[42;30m"
	ansiBgRed       = "\033[41;37m"
	ansiBgYellow    = "\033[43;30m"
	ansiBgCyan      = "\033[46;30m"
	ansiBgBlue      = "\033[44;37m"
)

func style(code, text string) string {
	if !colorEnabled() {
		return text
	}
	return code + text + ansiReset
}

func bold(s string) string       { return style(ansiBold, s) }
func dim(s string) string        { return style(ansiDim, s) }
func italic(s string) string     { return style(ansiItalic, s) }
func underline(s string) string  { return style(ansiUnderline, s) }
func red(s string) string        { return style(ansiRed, s) }
func green(s string) string      { return style(ansiGreen, s) }
func yellow(s string) string     { return style(ansiYellow, s) }
func blue(s string) string       { return style(ansiBlue, s) }
func magenta(s string) string    { return style(ansiMagenta, s) }
func cyan(s string) string       { return style(ansiCyan, s) }
func brightCyan(s string) string { return style(ansiBrightCyan, s) }
func brightBlue(s string) string { return style(ansiBrightBlue, s) }
func gray(s string) string       { return style(ansiGray, s) }

// Icons and Badges for TTY interaction
func iconCheck() string {
	if !colorEnabled() {
		return "[OK]"
	}
	return green("✔")
}

func iconCross() string {
	if !colorEnabled() {
		return "[FAIL]"
	}
	return red("✖")
}

func iconWarn() string {
	if !colorEnabled() {
		return "[WARN]"
	}
	return yellow("⚠")
}

func iconQuestion() string {
	if !colorEnabled() {
		return "?"
	}
	return brightCyan("?")
}

func iconArrow() string {
	if !colorEnabled() {
		return ">"
	}
	return brightCyan("❯")
}

func formatBanner() string {
	if !colorEnabled() {
		return "=== AGENTE ARQ (AGENTARCH) ==="
	}

	bgOrange := "\033[48;5;208;1;30m"
	reset := "\033[0m"

	title := "   ⚡ AGENTE ARQ  •  OPEN STANDARD FOR AI AGENTS (agentarch)   "
	pad := strings.Repeat(" ", len(title))

	lineTop := bgOrange + pad + reset
	lineMid := bgOrange + title + reset
	lineBot := bgOrange + pad + reset

	return fmt.Sprintf("\n%s\n%s\n%s\n", lineTop, lineMid, lineBot)
}

func formatCard(title string, bodyLines []string) string {
	if !colorEnabled() {
		out := "=== " + title + " ===\n"
		for _, l := range bodyLines {
			out += l + "\n"
		}
		return out
	}

	maxLen := len(title)
	for _, l := range bodyLines {
		raw := stripANSI(l)
		if len(raw) > maxLen {
			maxLen = len(raw)
		}
	}
	width := maxLen + 4

	top := cyan("┌─ ") + bold(title) + cyan(" "+strings.Repeat("─", width-len(title)-3)+"┐")
	bottom := cyan("└" + strings.Repeat("─", width) + "┘")

	var result []string
	result = append(result, top)
	for _, l := range bodyLines {
		rawLen := len(stripANSI(l))
		pad := width - rawLen - 2
		if pad < 0 {
			pad = 0
		}
		result = append(result, cyan("│ ") + l + strings.Repeat(" ", pad) + cyan("│"))
	}
	result = append(result, bottom)
	return strings.Join(result, "\n")
}

func stripANSI(str string) string {
	var sb strings.Builder
	inEsc := false
	for _, r := range str {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func formatHeader(title string) string {
	if !colorEnabled() {
		return "=== " + title + " ==="
	}
	return bold(brightCyan("━━━ " + title + " ━━━"))
}

func formatCmd(cmd string) string {
	if !colorEnabled() {
		return cmd
	}
	return bold(brightCyan(cmd))
}

func formatStep(num int, text string) string {
	if !colorEnabled() {
		return fmt.Sprintf("  %d. %s", num, text)
	}
	return fmt.Sprintf("  %s %s", brightCyan(fmt.Sprintf("%d.", num)), bold(text))
}

func animateSpinner(msg string, duration time.Duration) {
	if !colorEnabled() {
		fmt.Println(msg)
		return
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	stop := time.After(duration)
	i := 0
	for {
		select {
		case <-stop:
			fmt.Printf("\r\033[K%s %s\n", iconCheck(), msg)
			return
		default:
			fmt.Printf("\r\033[K%s %s", brightCyan(frames[i%len(frames)]), dim(msg))
			time.Sleep(80 * time.Millisecond)
			i++
		}
	}
}
