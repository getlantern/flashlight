package replica

import "time"

// This is supposed to mirror parts of SearchResultItem in replica-search.
type objectInfo struct {
	Link     string `json:"replica_link"`
	FileSize int64  `json:"file_size"`
	//MimeTypes    []string
	LastModified time.Time `json:"last_modified"`
}
