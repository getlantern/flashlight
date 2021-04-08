package desktopReplica

import (
	"mime"
	"path"
	"time"

	"github.com/getlantern/replica"
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
func (me *objectInfo) fromS3UploadMetaInfo(mi replica.UploadMetainfo, lastModified time.Time) error {
	filePath := mi.FilePath()
	*me = objectInfo{
		FileSize:     mi.TotalLength(),
		LastModified: lastModified,
		Link:         replica.CreateLink(mi.HashInfoBytes(), mi.Upload, filePath),
		DisplayName:  path.Join(filePath...),
		MimeTypes: func() []string {
			if len(filePath) == 0 {
				return nil
			}
			return []string{mime.TypeByExtension(path.Ext(filePath[len(filePath)-1]))}
		}(),
	}
	return nil
}
