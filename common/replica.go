package common

import (
	"strings"
)

func ReplicaEnabled() bool {
	return strings.Contains(Version, "replica")
}
