//go:build cgo
// +build cgo

package sqliteStorage

import (
	"github.com/anacrolix/squirrel"
)

type NewDirectStorageOpts = squirrel.NewCacheOpts
