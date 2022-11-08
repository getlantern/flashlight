package internal

import (
	"bytes"
	"encoding/gob"
	"github.com/OperatorFoundation/go-bloom"
	"hash/fnv"
	"log"
	"os"
	"sync"
)

// simply use Double FNV here as our Bloom Filter hash
func doubleFNV(b []byte) (uint64, uint64) {
	hx := fnv.New64()
	_, _ = hx.Write(b)
	x := hx.Sum64()
	hy := fnv.New64a()
	_, _ = hy.Write(b)
	y := hy.Sum64()
	return x, y
}

type BloomRing struct {
	SlotCapacity int
	SlotPosition int
	SlotCount    int
	EntryCounter int
	Slots        []bloom.Filter
	mutex        sync.RWMutex
}

func NewBloomRing(slot int, capacity int, falsePositiveRate float64) *BloomRing {
	// Calculate entries for each slot
	r := &BloomRing{
		SlotCapacity: capacity / slot,
		SlotCount:    slot,
		Slots:        make([]bloom.Filter, slot),
	}
	for i := 0; i < slot; i++ {
		r.Slots[i] = bloom.New(r.SlotCapacity, falsePositiveRate, doubleFNV)
	}
	return r
}

func LoadBloomRing(filePath string) (*BloomRing, error) {

	data, readError := os.ReadFile(filePath)
	if readError != nil {
		return nil, readError
	}

	buffer := bytes.NewBuffer(data)

	// Create a decoder and receive a value.
	decoder := gob.NewDecoder(buffer)
	var ring BloomRing
	decodeError := decoder.Decode(&ring)
	if decodeError != nil {
		log.Fatal("decode:", decodeError)
		return nil, decodeError
	}

	for i := 0; i < len(ring.Slots); i++ {
		slot := ring.Slots[i]
		filter, ok := slot.(*bloom.ClassicFilter)
		if ok {
			filter.H = doubleFNV
		}
	}

	return &ring, nil
}

func LoadOrCreateBloomRing(filePath string, slot int, capacity int, falsePositiveRate float64) *BloomRing {
	classicFilter := bloom.ClassicFilter{}
	gob.RegisterName("github.com/OperatorFoundation/go-bloom.ClassicFilter", &classicFilter)
	ring, loadError := LoadBloomRing(filePath)
	if loadError != nil {
		return NewBloomRing(slot, capacity, falsePositiveRate)
	} else {
		return ring
	}
}

func (r *BloomRing) Save(filePath string) error {
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	encodeError := enc.Encode(r)
	if encodeError != nil {
		log.Fatal("encode:", encodeError)
		return encodeError
	}

	data := buffer.Bytes()
	writeError := os.WriteFile(filePath, data, 0666)
	if writeError != nil {
		return writeError
	}

	return nil
}

func (r *BloomRing) Add(b []byte) {
	if r == nil {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.add(b)
}

func (r *BloomRing) add(b []byte) {
	slot := r.Slots[r.SlotPosition]
	if r.EntryCounter > r.SlotCapacity {
		// Move to next slot and reset
		r.SlotPosition = (r.SlotPosition + 1) % r.SlotCount
		slot = r.Slots[r.SlotPosition]
		slot.Reset()
		r.EntryCounter = 0
	}
	r.EntryCounter++
	slot.Add(b)
}

func (r *BloomRing) Test(b []byte) bool {
	if r == nil {
		return false
	}
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	test := r.test(b)
	return test
}

func (r *BloomRing) test(b []byte) bool {
	for _, s := range r.Slots {
		if s.Test(b) {
			return true
		}
	}
	return false
}

func (r *BloomRing) Check(b []byte) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.Test(b) {
		return true
	}
	r.Add(b)
	return false
}
