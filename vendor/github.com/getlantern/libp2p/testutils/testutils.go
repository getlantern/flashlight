package testutils

import (
	"context"
	cryptoRand "crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	mathRand "math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/traversal"
	"github.com/getlantern/libp2p/common"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

type DHTWrapperMock__DontReturnFreePeers struct {
}

func (d *DHTWrapperMock__DontReturnFreePeers) PutFreePeers(
	ctx context.Context,
	freePeers []*common.GenericFreePeer,
	privKeySeed []byte,
	salt string,
	seq int64,
) (krpc.ID, *traversal.Stats, error) {
	return krpc.ID{}, nil, nil
}

func (d *DHTWrapperMock__DontReturnFreePeers) GetFreePeers(
	ctx context.Context,
	targetAndSalt common.Bep44TargetAndSalt,
	lastChecksum *uint32,
) (
	freePeers []*common.GenericFreePeer,
	checksum uint32,
	hasNewPeers bool,
	err error) {
	return nil, 0, false, nil
}

func (d *DHTWrapperMock__DontReturnFreePeers) Close() {}

type DHTWrapperMock__AlwaysReturnPeers struct {
	FreePeers []*common.GenericFreePeer
	Checksum  *uint32
}

func (d *DHTWrapperMock__AlwaysReturnPeers) PutFreePeers(
	ctx context.Context,
	freePeers []*common.GenericFreePeer,
	privKeySeed []byte,
	salt string,
	seq int64,
) (krpc.ID, *traversal.Stats, error) {
	return krpc.ID{}, nil, nil
}

func (d *DHTWrapperMock__AlwaysReturnPeers) GetFreePeers(
	ctx context.Context,
	targetAndSalt common.Bep44TargetAndSalt,
	lastChecksum *uint32,
) (
	freePeers []*common.GenericFreePeer,
	checksum uint32,
	hasNewPeers bool,
	err error) {
	if d.Checksum != nil {
		checksum = *d.Checksum
		if lastChecksum != nil {
			if checksum == *lastChecksum {
				hasNewPeers = false
			} else {
				hasNewPeers = true
			}
		}
	} else {
		checksum = mathRand.Uint32()
		hasNewPeers = true
	}
	return d.FreePeers, checksum, hasNewPeers, nil
}

func (d *DHTWrapperMock__AlwaysReturnPeers) Close() {}

func GenerateRandomInfohashes(t *testing.T, maxCount int) []string {
	a := []string{}
	for i := 0; i < maxCount; i++ {
		b := make([]byte, 40) // size doesn't matter
		_, err := cryptoRand.Read(b)
		require.NoError(t, err)
		sum := sha1.New().Sum(b)
		h := hex.EncodeToString(sum)
		a = append(a, h)
	}
	return a
}

func GenerateRandomFreePeers(
	t *testing.T,
	maxCount int,
) []*common.GenericFreePeer {
	arr := []*common.GenericFreePeer{}
	for i := 0; i < maxCount; i++ {
		p := &common.GenericFreePeer{
			IP: net.ParseIP(fmt.Sprintf(
				"%d.%d.%d.%d",
				mathRand.Intn(256),
				mathRand.Intn(256),
				mathRand.Intn(256),
				mathRand.Intn(256),
			)),
			Port:               mathRand.Intn(65535),
			PubCertFingerprint: nil,
		}
		arr = append(arr, p)
	}
	return arr
}

func InitTestRegistrar(t *testing.T,
	onRegisterHandler func(w http.ResponseWriter, r *http.Request),
) *httptest.Server {
	r := mux.NewRouter()
	r.HandleFunc("/register", onRegisterHandler).Methods("POST")
	return httptest.NewServer(r)
}
