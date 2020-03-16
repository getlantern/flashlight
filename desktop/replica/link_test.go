package replica

import (
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
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
			"&dn=nice+name"+
			"&so=0"+ // Not sure if we can rely on the ordering of params, hope so.
			"&tr=http%3A%2F%2Fs3-tracker.ap-southeast-1.amazonaws.com%3A6969%2Fannounce"+
			"&xs=replica%3Abig+long+uuid%2Fherp.txt", link)
}

// This is to check that s3KeyFromMagnet doesn't return an error if there's no replica xs parameter.
// This is valid for Replica magnet links that don't refer to items on S3.
func TestS3KeyFromMagnetMissingXs(t *testing.T) {
	m, err := metainfo.ParseMagnetURI("magnet:?xt=urn:btih:b84d0051d6cc64eb48bf8c47dd44320f69c17544&dn=Test+Drive+Unlimited+ReincarnaTion%2FTest+Drive+Unlimited+ReincarnaTion.exe&so=0")
	require.NoError(t, err)
	s3Key, err := s3KeyFromMagnet(m)
	require.NoError(t, err)
	require.EqualValues(t, "", s3Key)
}
