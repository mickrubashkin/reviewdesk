package auth

import (
	"errors"
	"regexp"
	"strings"
)

var errGenericAddress = errors.New("shared addresses (sales@, partners@, info@, noreply@) cannot sign in; use a personal work address")

// genericAddressPattern matches shared team mailboxes such as sales@,
// sales-de@ or no-reply@, but not personal names that merely start the same
// way (salesky@).
var genericAddressPattern = regexp.MustCompile(`^(sales|partners|noreply|no-reply|info)([^a-z]|$)`)

// isGenericAddress reports whether email is a shared mailbox rather than a
// person; those never get access.
func isGenericAddress(email string) bool {
	local, _, ok := strings.Cut(strings.ToLower(strings.TrimSpace(email)), "@")
	return ok && genericAddressPattern.MatchString(local)
}
