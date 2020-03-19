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
	Link         string    `json:"replica_link"`
	FileSize     int64     `json:"file_size"`
	MimeTypes    []string  `json:"mime_types"`
	LastModified time.Time `json:"last_modified"`
	DisplayName  string    `json:"display_name"`
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
