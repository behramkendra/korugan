// Package obs holds observability plumbing: structured logging with
// secret redaction. OpenTelemetry wiring lands here in a later phase.
package obs

import (
	"log/slog"
	"os"
	"regexp"
	"strings"
)

// secretPatterns match common credential shapes. Anything matching is
// replaced before a log line is emitted. Kept intentionally aggressive:
// a false positive costs a garbled log line, a false negative leaks a key.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)[a-z0-9._\-]{8,}`),
	regexp.MustCompile(`sk-[a-zA-Z0-9._\-]{8,}`),                                         // OpenAI/Anthropic/OpenRouter style
	regexp.MustCompile(`(?i)(password|passwd|secret|token|apikey|api_key)=([^&\s]{4,})`), // URL/kv shapes
	regexp.MustCompile(`postgres(ql)?://[^:/\s]+:([^@\s]+)@`),                            // DSN password segment
}

// Redact scrubs credential-shaped substrings from s.
func Redact(s string) string {
	out := s
	out = secretPatterns[0].ReplaceAllString(out, "${1}[REDACTED]")
	out = secretPatterns[1].ReplaceAllString(out, "sk-[REDACTED]")
	out = secretPatterns[2].ReplaceAllString(out, "${1}=[REDACTED]")
	out = secretPatterns[3].ReplaceAllString(out, "postgres${1}://[REDACTED]@")
	return out
}

// NewLogger builds the process logger. Level accepts debug|info|warn|error.
func NewLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Value.Kind() == slog.KindString {
				a.Value = slog.StringValue(Redact(a.Value.String()))
			}
			return a
		},
	})
	return slog.New(h)
}
