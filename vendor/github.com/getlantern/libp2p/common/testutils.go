package common

import (
	cryptoRand "crypto/rand"
	"encoding/hex"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/stretchr/testify/require"
)

// GenerateRandomizedBep44TargetAndSalt fetches a randomized
// Bep44TargetAndSalt. It's used exclusively in tests.
func GenerateRandomizedBep44TargetAndSalt(
	t require.TestingT,
	count int,
) []Bep44TargetAndSalt {
	arr := []Bep44TargetAndSalt{}
	for i := 0; i < count; i++ {
		target := make([]byte, 20)
		_, err := cryptoRand.Read(target)
		require.NoError(t, err)
		var id krpc.ID
		copy(id[:], target)

		salt := make([]byte, 10)
		_, err = cryptoRand.Read(salt)
		require.NoError(t, err)
		arr = append(arr, Bep44TargetAndSalt{
			Target: id,
			Salt:   hex.EncodeToString(salt),
		})
	}
	return arr
}
