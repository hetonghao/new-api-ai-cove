package common

import (
	"net/http"
	"regexp"
	"strings"
)

var codexClientVersion = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// IsCodexClientRequest follows Sub2API's default codex_cli_only policy:
// https://github.com/ranxi2001/sub2api/blob/9fcfe25d600a331abcc28a1788e597e5542d4cd3/backend/internal/service/openai_client_restriction_detector.go
// These caller-supplied signals classify clients; they do not authenticate them.
func IsCodexClientRequest(request *http.Request) bool {
	if request == nil {
		return false
	}
	ua := strings.ToLower(strings.TrimSpace(request.UserAgent()))
	client, version, found := strings.Cut(ua, "/")
	if end := strings.IndexAny(version, " ("); end >= 0 {
		version = version[:end]
	}
	if !found || !codexClientVersion.MatchString(strings.TrimSpace(version)) {
		return false
	}
	matched := (client == strings.TrimSpace(client) && isOfficialCodexOriginator(client)) || strings.HasPrefix(ua, "codex ") || isOfficialCodexOriginator(request.Header.Get("originator"))
	if !matched {
		if start := strings.LastIndex(ua, "("); start >= 0 {
			trailer := ua[start+1:]
			if inner, _, closed := strings.Cut(trailer, ")"); closed {
				name, _, _ := strings.Cut(inner, ";")
				matched = isOfficialCodexOriginator(name)
			}
		}
	}
	if !matched {
		return false
	}
	for name := range request.Header {
		if strings.HasPrefix(strings.ToLower(name), "x-codex-") {
			return true
		}
	}
	return false
}

func isOfficialCodexOriginator(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "codex_cli_rs", "codex-tui", "codex_vscode", "codex_vscode_copilot", "codex_app", "codex_chatgpt_desktop", "codex_atlas", "codex_exec", "codex_sdk_ts":
		return true
	default:
		return strings.HasPrefix(value, "codex ")
	}
}
