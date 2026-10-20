package workflow

import (
	"regexp"
	"strconv"
)

// MaxNameLength is the one numeric source of the run-name length policy —
// names run 1..MaxNameLength characters. It lives in the root package because
// the message vocabulary (MsgNameInvalid) renders it; the state package's
// names index consumes it from here.
const MaxNameLength = 64

// ValidName is the well-formedness contract for a run name: 1-MaxNameLength
// chars, leading letter or underscore, then letters, digits, hyphens and
// underscores.
var ValidName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]{0,` + strconv.Itoa(MaxNameLength-1) + `}$`)

// IsValidName reports whether name satisfies ValidName.
func IsValidName(name string) bool {
	return ValidName.MatchString(name)
}
