package webapi

import (
	"regexp"
	"strings"
)

// JobCause is why a job's last attempt failed, where kritique can tell from
// the error: what the dashboard says in place of the error's own text.
type JobCause string

// CauseForgeUnavailable is the forge, GitHub, Forgejo or Gitea, not
// answering a call, or answering it with a server error: nothing kritique
// or its admin can fix, and the job's next attempt may well pass.
const CauseForgeUnavailable JobCause = "forge_unavailable"

// forgeServerError is go-github's text for a 5xx response, and the Forgejo
// client's: the request, then the status.
var forgeServerError = regexp.MustCompile(`: 5\d\d `)

// forgeUnanswered are the texts of a call that got no response.
var forgeUnanswered = []string{
	"context deadline exceeded", "timeout", "connection refused", "connection reset", "no such host", "EOF",
}

// forgePrefixes start the errors of the forge clients' calls: GitHub's,
// and the one client Forgejo and Gitea share.
var forgePrefixes = []string{"github: ", "forgejo: "}

// jobCause reads a cause from a job's last error. River keeps only the
// error's text, so a forge client's prefix is what says the error is the
// forge's.
func jobCause(lastError string) JobCause {
	var forge string
	for _, prefix := range forgePrefixes {
		if _, rest, ok := strings.Cut(lastError, prefix); ok {
			forge = rest
			break
		}
	}
	if forge == "" {
		return ""
	}
	if forgeServerError.MatchString(forge) {
		return CauseForgeUnavailable
	}
	for _, text := range forgeUnanswered {
		if strings.Contains(forge, text) {
			return CauseForgeUnavailable
		}
	}
	return ""
}
