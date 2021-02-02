package common

import "strconv"

var (
	EnableReplicaFeatures = "false"
	EnableReplica         = false
)

func init() {
	EnableReplicaFeatures, _ = strconv.ParseBool(EnableReplicaFeatures)
}
