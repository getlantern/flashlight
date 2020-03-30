package replica

import (
	"mime"
	"path"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

// This is supposed to mirror parts of SearchResultItem in replica-search.
// https://github.com/getlantern/replica-search/blob/a9975d98e2b40d7c8087dc27d434cc4bb13299fe/src/server.rs#L9-L24
type objectInfo struct {
	Link         string    `json:"replicaLink"`
	FileSize     int64     `json:"fileSize"`
	MimeTypes    []string  `json:"mimeTypes"`
	LastModified time.Time `json:"lastModified"`
	DisplayName  string    `json:"displayName"`
}

// Inits from a BitTorrent metainfo that must contain a valid info.
func (me *objectInfo) fromS3UploadMetaInfo(mi *metainfo.MetaInfo, lastModified time.Time) {
	info, err := mi.UnmarshalInfo()
	if err != nil {
		panic(err) // Don't pass a bad metainfo...
	}
	dn := displayNameFromInfoName(info.Name)
	*me = objectInfo{
		FileSize:     info.TotalLength(),
		LastModified: lastModified,
		Link:         createLink(mi.HashInfoBytes(), s3KeyFromInfoName(info.Name), dn),
		DisplayName:  dn,
		MimeTypes:    []string{mime.TypeByExtension(path.Ext(dn))},
	}
}
