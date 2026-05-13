package magic8ball

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// agentSpec describes how to invoke one AI coding-agent CLI so it returns
// a short magic-8-ball answer.
type agentSpec struct {
	// name is the human-readable label shown in the UI ("Claude", …).
	name string
	// cmd is the binary that must be present on PATH (looked up via
	// exec.LookPath).
	cmd string
	// args builds the argv (minus cmd itself) for a one-shot
	// non-interactive prompt. The prompt is passed as a single argv so
	// the user's shell never sees it — no quoting hazards.
	args func(prompt string) []string
}

// agents is the priority-ordered list of coding agents the magic 8 ball
// will try. The first one whose binary is on PATH wins. Order matches
// the user's stated preference: Claude → Codex → Gemini → Copilot.
var agents = []agentSpec{
	{
		name: "Claude",
		cmd:  "claude",
		args: func(p string) []string { return []string{"-p", p} },
	},
	{
		name: "Codex",
		cmd:  "codex",
		args: func(p string) []string { return []string{"exec", p} },
	},
	{
		name: "Gemini",
		cmd:  "gemini",
		args: func(p string) []string { return []string{"-p", p} },
	},
	{
		name: "Copilot",
		cmd:  "copilot",
		args: func(p string) []string { return []string{"-p", p} },
	},
}

// agentRequestTimeout caps how long we'll wait on any one agent CLI.
// Coding agents can be slow on first invocation; 60s is generous but
// keeps the game from hanging forever if something is wedged.
const agentRequestTimeout = 60 * time.Second

// promptTmpl is the instruction sent to every agent. We keep the
// format constraints (length + no markdown/quotes/preamble) because
// agents otherwise reply with multi-sentence explanations, but we
// deliberately *don't* enumerate canned 8-ball phrases — listing
// examples collapses the response space onto those few patterns. The
// goal is for the agent to actually *answer* the question with its own
// personality.
const promptTmpl = "You are a mystical, opinionated magic 8 ball with personality. " +
	"Give a punchy reply of 1 to 5 words to the user's question. " +
	"You may be cryptic, confident, playful, ominous, sassy, encouraging, or absurd as fits the question — " +
	"do not feel obligated to stick to traditional 8 ball phrases. Surprise the asker, but stay relevant to what they asked. " +
	"Reply with ONLY the answer text — no quotes, no markdown, no code fences, no explanation, no preamble. " +
	"Question: %s"

// agentResult is what the async ask returns: either a cleaned answer
// labelled with the agent that produced it, or an error.
type agentResult struct {
	answer string
	agent  string
	err    error
}

// errNoAgent is returned when no agent in the list is on PATH (or all
// of them errored).
var errNoAgent = errors.New("no supported AI agent CLI found on PATH")

// detectAgent returns the highest-priority agent whose binary is on
// PATH right now, or false if none are installed. The UI uses this to
// label the input screen ("powered by Claude").
func detectAgent() (agentSpec, bool) {
	for _, a := range agents {
		if _, err := exec.LookPath(a.cmd); err == nil {
			return a, true
		}
	}
	return agentSpec{}, false
}

// askAgent tries each installed agent in priority order and returns the
// first non-empty cleaned response. If nothing on PATH succeeds, the
// returned agentResult carries errNoAgent.
//
// Designed to run from a goroutine: writes a single value to a result
// channel. Honour ctx cancellation between attempts so ESC during shake
// can abort cleanly.
func askAgent(ctx context.Context, question string) agentResult {
	tried := 0
	for _, a := range agents {
		if _, err := exec.LookPath(a.cmd); err != nil {
			continue
		}
		tried++
		if err := ctx.Err(); err != nil {
			return agentResult{err: err}
		}
		cctx, cancel := context.WithTimeout(ctx, agentRequestTimeout)
		prompt := fmt.Sprintf(promptTmpl, question)
		cmd := exec.CommandContext(cctx, a.cmd, a.args(prompt)...)
		out, err := cmd.Output()
		cancel()
		if err != nil {
			// Fall through to the next agent.
			continue
		}
		cleaned := cleanResponse(string(out))
		if cleaned == "" {
			continue
		}
		return agentResult{answer: cleaned, agent: a.name}
	}
	if tried == 0 {
		return agentResult{err: errNoAgent}
	}
	return agentResult{err: fmt.Errorf("all installed agents failed to produce a response")}
}

// ansiRe matches CSI escape sequences (e.g. spinner refreshes) so we
// can strip them before picking the answer line.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// cleanResponse normalises whatever the CLI printed into a short
// uppercase answer that will fit the 8 ball window. The strategy:
//
//  1. Strip ANSI escape sequences.
//  2. Pick the first non-empty trimmed line. Agents in one-shot/print
//     mode reliably put the answer on its own line.
//  3. Strip wrapping quotes and trailing sentence punctuation.
//  4. Drop non-printable / non-ASCII bytes leftover from spinners.
//  5. Cap at 30 characters and uppercase for the 8-ball aesthetic.
func cleanResponse(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	if strings.TrimSpace(s) == "" {
		return ""
	}
	var picked string
	for ln := range strings.SplitSeq(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		picked = ln
		break
	}
	if picked == "" {
		return ""
	}
	// Strip a single pair of matching wrapping quotes (ASCII or curly).
	picked = trimWrappingQuotes(picked)
	// Trim trailing periods and whitespace — but leave `!` and `?` so
	// "Absolutely!" and "Who knows?" keep their punch.
	picked = strings.TrimRightFunc(picked, func(r rune) bool {
		return r == '.' || unicode.IsSpace(r)
	})
	// Keep printable ASCII only (canvas font supports A-Z, 0-9, basic punctuation).
	var b strings.Builder
	for _, r := range picked {
		if r >= 32 && r < 127 {
			b.WriteRune(r)
		}
	}
	picked = strings.TrimSpace(b.String())
	if picked == "" {
		return ""
	}
	// 36 fits a 5-word reply at average word length comfortably inside
	// the triangle window after wrapping.
	const maxLen = 36
	if len(picked) > maxLen {
		picked = strings.TrimSpace(picked[:maxLen])
	}
	return strings.ToUpper(picked)
}

// trimWrappingQuotes removes one matching pair of surrounding ASCII or
// Unicode curly quotes from s. Used in cleanResponse so a reply like
// `"Yes"` reads as `YES`.
func trimWrappingQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	pairs := [][2]string{
		{`"`, `"`},
		{`'`, `'`},
		{"“", "”"}, // “ ”
		{"‘", "’"}, // ‘ ’
	}
	for _, p := range pairs {
		if strings.HasPrefix(s, p[0]) && strings.HasSuffix(s, p[1]) {
			return s[len(p[0]) : len(s)-len(p[1])]
		}
	}
	return s
}
