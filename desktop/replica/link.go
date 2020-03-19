package replica

import (
	"net/url"
	"strings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

func createLink(ih torrent.InfoHash, s3Key, name string) string {
	return metainfo.Magnet{
		InfoHash:    ih,
		DisplayName: name,
		Trackers:    []string{"http://s3-tracker.ap-southeast-1.amazonaws.com:6969/announce"},
		Params: url.Values{
			"as": {"https://getlantern-replica.s3-ap-southeast-1.amazonaws.com" + s3Key},
			"xs": {(&url.URL{Scheme: "replica", Opaque: s3Key}).String()},
			// This might technically be more correct, but I couldn't find any torrent client that
			// supports it. Make sure to change any assumptions about "xs" before changing it.
			//"xs": {"https://getlantern-replica.s3-ap-southeast-1.amazonaws.com" + s3Key + "?torrent"},

			// Since S3 key is provided, we know that it must be a single-file torrent.
			"so": {"0"},
		},
	}.String()
}

// This reverses s3 key to info name change that AWS makes in its ObjectTorrent metainfos.
func s3KeyFromInfoName(name string) string {
	return strings.Replace(name, "_", "/", 1)
}

// Retrieve the original, user or file-system provided file name, before changes made by AWS.
func displayNameFromInfoName(name string) string {
	ss := strings.SplitN(name, "_", 2)
	if len(ss) > 1 {
		return ss[1]
	}
	return ss[0]
}

// See createLink.
func s3KeyFromMagnet(m metainfo.Magnet) (string, error) {
	// url.Parse("") doesn't return an error! (which is currently what we want here).
	u, err := url.Parse(m.Params.Get("xs"))
	if err != nil {
		return "", err
	}
	if u.Opaque != "" {
		return u.Opaque, nil
	}
	return u.Path, nil
}
