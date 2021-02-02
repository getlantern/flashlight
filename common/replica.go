package common

import "strconv"

var (
	EnableReplicaFeatures = "false"
	EnableReplica         = false
)

func init() {
	EnableReplica, _ = strconv.ParseBool(EnableReplicaFeatures)
}
