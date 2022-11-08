package persistentcache

import (
	"os"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/getlantern/dnsgrab/internal"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("dnsgrab.persistentcache")

	namesByIPBucket = []byte("namesByIP")
	ipsByNameBucket = []byte("ipsByName")
)

type entry []byte

func newEntry(value []byte) []byte {
	e := make(entry, 8+len(value))
	copy(e[8:], value)
	e.mark()
	return e
}

// We have to copy any byte arrays returned by bolt since those are only valid during the lifetime of a transaction.
// See https://pkg.go.dev/go.etcd.io/bbolt/#Bucket.Get
func (e entry) copy() entry {
	e2 := make(entry, len(e))
	copy(e2, e)
	return e2
}

func (e entry) mark() {
	now := uint64(time.Now().UnixNano())
	internal.Endianness.PutUint64(e, now)
}

func (e entry) expired(maxAge time.Duration) bool {
	elapsed := time.Duration(time.Now().UnixNano()) - e.tsNanos()
	expired := elapsed > maxAge
	if expired {
		log.Debugf("%v exceeds max age of %v", elapsed, maxAge)
	}
	return expired
}

func (e entry) tsNanos() time.Duration {
	return time.Duration(internal.Endianness.Uint64(e))
}

func (e entry) value() []byte {
	// We have to copy any byte arrays returned by bolt since those are only valid during the lifetime of a transaction.
	// See https://pkg.go.dev/go.etcd.io/bbolt/#Bucket.Get
	return copySlice(e[8:])
}

func copySlice(b []byte) []byte {
	result := make([]byte, len(b))
	copy(result, b)
	return result
}

// PersistentCache is an age bounded on-disk cache
type PersistentCache struct {
	db *bolt.DB

	maxAge time.Duration
}

func New(filename string, maxAge time.Duration) (*PersistentCache, error) {
	db, err := bolt.Open(filename, 0644, nil)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		// create buckets if necessary
		namesByIP, err := tx.CreateBucketIfNotExists(namesByIPBucket)
		if err != nil {
			return err
		}

		ipsByName, err := tx.CreateBucketIfNotExists(ipsByNameBucket)
		if err != nil {
			return err
		}

		// delete expired entries
		namesDeleted := 0
		ipsDeleted := 0

		err = namesByIP.ForEach(func(k, v []byte) error {
			if entry(v).expired(maxAge) {
				namesDeleted++
				return namesByIP.Delete(k)
			}
			return nil
		})
		if err != nil {
			return err
		}

		err = ipsByName.ForEach(func(k, v []byte) error {
			if entry(v).expired(maxAge) {
				ipsDeleted++
				return ipsByName.Delete(k)
			}
			return nil
		})
		if err != nil {
			return err
		}

		log.Debugf("Deleted %d names and %d ips", namesDeleted, ipsDeleted)

		// initialize sequence if necessary
		seq := ipsByName.Sequence()
		if seq == 0 {
			// initialize sequence to MinIP
			ipsByName.SetSequence(uint64(internal.MinIP - 1)) // we subtract 1 so that the next call to NextSequence returns MinIP
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &PersistentCache{
		db:     db,
		maxAge: maxAge,
	}, nil
}

// Close closes the persistent cache
func (cache *PersistentCache) Close() error {
	return cache.db.Close()
}

func (cache *PersistentCache) NameByIP(ip []byte) (name string, found bool) {
	cache.update(func(namesByIP *bolt.Bucket, ipsByName *bolt.Bucket) error {
		e := entry(namesByIP.Get(ip))
		if e == nil {
			return nil
		}

		_name := e.value()
		if !e.expired(cache.maxAge) {
			name = string(_name)
			found = true
		} else {
			namesByIP.Delete(ip)
			ipsByName.Delete(_name)
			name, found = "", false
		}
		return nil
	})
	return
}

func (cache *PersistentCache) IPByName(name string) (ip []byte, found bool) {
	cache.update(func(namesByIP *bolt.Bucket, ipsByName *bolt.Bucket) error {
		_name := []byte(name)
		e := entry(ipsByName.Get(_name))
		if e == nil {
			return nil
		}

		ip = e.value()
		if !e.expired(cache.maxAge) {
			found = true
		} else {
			if err := ipsByName.Delete(_name); err != nil {
				return nil
			}
			if err := namesByIP.Delete(ip); err != nil {
				return nil
			}
			ip, found = nil, false
		}
		return nil
	})
	return
}

func (cache *PersistentCache) Add(name string, ip []byte) {
	cache.update(func(namesByIP *bolt.Bucket, ipsByName *bolt.Bucket) error {
		nameBytes := []byte(name)
		err := namesByIP.Put(ip, newEntry(nameBytes))
		if err != nil {
			return err
		}
		return ipsByName.Put(nameBytes, newEntry(ip))
	})
}

func (cache *PersistentCache) MarkFresh(name string, ip []byte) {
	cache.update(func(namesByIP *bolt.Bucket, ipsByName *bolt.Bucket) error {
		_name := []byte(name)
		nameEntry := entry(namesByIP.Get(ip)).copy()
		ipEntry := entry(ipsByName.Get(_name)).copy()
		nameEntry.mark()
		ipEntry.mark()
		if err := namesByIP.Put(ip, nameEntry); err != nil {
			return err
		}
		return ipsByName.Put(_name, ipEntry)
	})
}

func (cache *PersistentCache) NextSequence() (next uint32) {
	err := cache.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(ipsByNameBucket)
		_next, err := bucket.NextSequence()
		if err != nil {
			return err
		}
		if _next > uint64(internal.MaxIP) {
			err = bucket.SetSequence(uint64(internal.MinIP))
			if err != nil {
				return err
			}
			_next = uint64(internal.MinIP)
		}
		next = uint32(_next)
		return nil
	})
	if err != nil {
		// This should never happen and likely means the db is corrupt. Delete the database and then panic.
		os.RemoveAll(cache.db.Path())
		panic(err)
	}
	return
}

func (cache *PersistentCache) update(fn func(namesByIP *bolt.Bucket, ipsByName *bolt.Bucket) error) {
	err := cache.db.Update(func(tx *bolt.Tx) error {
		namesByIP := tx.Bucket(namesByIPBucket)
		ipsByName := tx.Bucket(ipsByNameBucket)
		return fn(namesByIP, ipsByName)
	})
	if err != nil {
		panic(err)
	}
}
