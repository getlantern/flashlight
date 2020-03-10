package replica

import (
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/stretchr/testify/require"
)

func TestCreateLink(t *testing.T) {
	const infoHashHex = "deadbeefc0ffeec0ffeedeadbeefc0ffeec0ffee"
	var infoHash torrent.InfoHash
	require.NoError(t, infoHash.FromHexString(infoHashHex))
	link := createLink(infoHash, "big long uuid/herp.txt", "nice name")
	require.EqualValues(t,
		"magnet:?xt=urn:btih:deadbeefc0ffeec0ffeedeadbeefc0ffeec0ffee"+
			"&as=https%3A%2F%2Fgetlantern-replica.s3-ap-southeast-1.amazonaws.combig+long+uuid%2Fherp.txt"+
			"&dn=nice+name&tr=http%3A%2F%2Fs3-tracker.ap-southeast-1.amazonaws.com%3A6969%2Fannounce"+
			"&xs=replica%3Abig+long+uuid%2Fherp.txt", link)
}
