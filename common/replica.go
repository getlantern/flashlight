package common

import "strconv"

var (
	// Enable replica related features via the REPLICA build time
	// variable
	EnableReplicaFeatures = "false"
	EnableReplica         = false
)

func init() {
	EnableReplica, _ = strconv.ParseBool(EnableReplicaFeatures)
}
