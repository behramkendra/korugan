package obs

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	cases := map[string]string{
		"Authorization: Bearer abc123def456":                    "abc123def456",
		"key sk-or-v1-verysecretkeymaterial ok":                 "verysecret",
		"dsn postgres://korugan:hunter2secret@127.0.0.1:5432/x": "hunter2secret",
		"POST /login?password=topsecret123&user=a":              "topsecret123",
		"api_key=deadbeefcafe1234 in config":                    "deadbeefcafe1234",
	}
	for in, mustHide := range cases {
		out := Redact(in)
		if strings.Contains(out, mustHide) {
			t.Errorf("Redact(%q) = %q — still leaks %q", in, out, mustHide)
		}
		if !strings.Contains(out, "REDACTED") {
			t.Errorf("Redact(%q) = %q — no redaction marker", in, out)
		}
	}
	if got := Redact("plain informational line"); got != "plain informational line" {
		t.Errorf("clean line altered: %q", got)
	}
}
