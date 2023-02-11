package common

import (
	"encoding/json"

	"github.com/anacrolix/dht/v2/krpc"
)

type Bep44TargetAndSalt struct {
	Target krpc.ID
	Salt   string
}

func (t Bep44TargetAndSalt) String() string {
	return t.Target.String() + ":" + t.Salt
}

func (a Bep44TargetAndSalt) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}
