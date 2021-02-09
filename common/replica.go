package common

import "strconv"

var (
	// Enable yinbi wallet related features via the YINBI env var
	EnableReplicaFeatures = "false"
	EnableReplica         = false
)

func init() {
	EnableReplica, _ = strconv.ParseBool(EnableReplicaFeatures)
}
