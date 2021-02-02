package common

import "strconv"

var (
	// Enable Replica related features via the
	// REPLICA env var
	EnableReplicaFeature = "false"
	EnableReplica        = false
)

func init() {
	EnableYinbiFeatures, _ = strconv.ParseBool(EnableReplicaFeature)
}
