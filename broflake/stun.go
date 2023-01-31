package broflake

import (
	"math/rand"
	"time"
)

type stunBatchFunc func(uint32) ([]string, error)

func newRandomSTUNs(servers []string) stunBatchFunc {
	r := &randomSTUNs{
		servers: servers,
		prng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	return r.GetBatch
}

type randomSTUNs struct {
	servers []string
	prng    *rand.Rand
}

func (s *randomSTUNs) GetBatch(size uint32) ([]string, error) {
	if size > uint32(len(s.servers)) {
		size = uint32(len(s.servers))
	}
	indices := s.prng.Perm(len(s.servers))[:size]
	batch := make([]string, 0, len(indices))
	for _, i := range indices {
		batch = append(batch, s.servers[i])
	}
	return batch, nil
}
