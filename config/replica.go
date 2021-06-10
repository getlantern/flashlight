package config

import (
	"fmt"
)

type ReplicaConfig struct {
	// Use infohash and old-style prefixing simultaneously for now. Later, the old-style can be removed.
	WebseedBaseUrls []string
	Trackers        []string
	StaticPeerAddrs []string
	// Merged with the webseed URLs when the metadata and data buckets are merged.
	MetadataBaseUrls       []string
	// This will vary by region in the future
	ReplicaServiceEndpoint string
}

func (gc *ReplicaConfig) MetainfoUrls(prefix string) (ret []string) {
	for _, s := range gc.WebseedBaseUrls {
		ret = append(ret, fmt.Sprintf("%s%s/torrent", s, prefix))
	}
	return
}

func (gc *ReplicaConfig) WebseedUrls(prefix string) (ret []string) {
	for _, s := range gc.WebseedBaseUrls {
		ret = append(ret, fmt.Sprintf("%s%s/data/", s, prefix))
	}
	return
}
