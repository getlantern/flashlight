// Package bytemap provides a map[string]interface{} encoded as a byte slice.
package bytemap

import (
	"bytes"
	"encoding/binary"
	"math"
	"sort"
	"time"
)

const (
	TypeNil = iota
	TypeBool
	TypeByte
	TypeUInt16
	TypeUInt32
	TypeUInt64
	TypeInt8
	TypeInt16
	TypeInt32
	TypeInt64
	TypeInt
	TypeFloat32
	TypeFloat64
	TypeString
	TypeTime
	TypeUInt
	TypeBytes
	TypeFloat64s
	TypeInts
)

const (
	SizeKeyLen      = 2
	SizeValueType   = 1
	SizeValueOffset = 4
)

var (
	enc = binary.LittleEndian
)

// ByteMap is an immutable map[string]interface{} backed by a byte array.
type ByteMap []byte

// New creates a new ByteMap from the given map
func New(m map[string]interface{}) ByteMap {
	return Build(func(cb func(string, interface{})) {
		for key, value := range m {
			cb(key, value)
		}
	}, func(key string) interface{} {
		return m[key]
	}, false)
}

// NewFloat creates a new ByteMap from the given map
func NewFloat(m map[string]float64) ByteMap {
	return Build(func(cb func(string, interface{})) {
		for key, value := range m {
			cb(key, value)
		}
	}, func(key string) interface{} {
		return m[key]
	}, false)
}

// FromSortedKeysAndValues constructs a ByteMap from sorted keys and values.
func FromSortedKeysAndValues(keys []string, values []interface{}) ByteMap {
	return Build(func(cb func(string, interface{})) {
		for i, key := range keys {
			cb(key, values[i])
		}
	}, nil, true)
}

// FromSortedKeysAndFloats constructs a ByteMap from sorted keys and float values.
func FromSortedKeysAndFloats(keys []string, values []float64) ByteMap {
	return Build(func(cb func(string, interface{})) {
		for i, key := range keys {
			cb(key, values[i])
		}
	}, nil, true)
}

// Build builds a new ByteMap using a function that iterates over all included
// key/value paris and another function that returns the value for a given key/
// index. If iteratesSorted is true, then the iterate order of iterate is
// considered to be in lexicographically sorted order over the keys and is
// stable over multiple invocations, and valueFor is not needed.
func Build(iterate func(func(string, interface{})), valueFor func(string) interface{}, iteratesSorted bool) ByteMap {
	keysLen := 0
	valuesLen := 0

	recordKey := func(key string, value interface{}) {
		valLen := encodedLength(value)
		keysLen += len(key) + SizeKeyLen + SizeValueType
		if valLen > 0 {
			keysLen += SizeValueOffset
		}
		valuesLen += valLen
	}

	var finalIterate func(func(string, interface{}))

	if iteratesSorted {
		iterate(func(key string, value interface{}) {
			recordKey(key, value)
		})
		finalIterate = iterate
	} else {
		sortedKeys := make([]string, 0, 10)
		iterate(func(key string, value interface{}) {
			sortedKeys = append(sortedKeys, key)
			recordKey(key, value)
		})
		sort.Strings(sortedKeys)

		finalIterate = func(cb func(string, interface{})) {
			for _, key := range sortedKeys {
				cb(key, valueFor(key))
			}
		}
	}

	startOfValues := keysLen
	bm := make(ByteMap, startOfValues+valuesLen)
	keyOffset := 0
	valueOffset := startOfValues
	finalIterate(func(key string, value interface{}) {
		keyLen := len(key)
		enc.PutUint16(bm[keyOffset:], uint16(keyLen))
		copy(bm[keyOffset+SizeKeyLen:], key)
		keyOffset += SizeKeyLen + keyLen
		t, n := encodeValue(bm[valueOffset:], value)
		bm[keyOffset] = t
		keyOffset += SizeValueType
		if t != TypeNil {
			enc.PutUint32(bm[keyOffset:], uint32(valueOffset))
			keyOffset += SizeValueOffset
			valueOffset += n
		}
	})

	return bm
}

// Get gets the value for the given key, or nil if the key is not found.
func (bm ByteMap) Get(key string) interface{} {
	keyBytes := []byte(key)
	keyOffset := 0
	firstValueOffset := 0
	for {
		keyLen, ok := bm.uint16At(keyOffset)
		if !ok {
			return nil
		}
		keyOffset += SizeKeyLen
		keysMatch := bm.compareAt(keyOffset, keyBytes)
		keyOffset += keyLen
		t, ok := bm.byteAt(keyOffset)
		if !ok {
			return nil
		}
		keyOffset += SizeValueType
		if t == TypeNil {
			if keysMatch {
				return nil
			}
		} else {
			valueOffset, ok := bm.uint32At(keyOffset)
			if !ok {
				return nil
			}
			if firstValueOffset == 0 {
				firstValueOffset = valueOffset
			}
			if keysMatch {
				return bm.decodeValueAt(valueOffset, t)
			}
			keyOffset += SizeValueOffset
		}
		if firstValueOffset > 0 && keyOffset >= firstValueOffset {
			break
		}
	}
	return nil
}

// GetBytes gets the bytes slice for the given key, or nil if the key is not
// found.
func (bm ByteMap) GetBytes(key string) []byte {
	keyBytes := []byte(key)
	keyOffset := 0
	firstValueOffset := 0
	for {
		keyLen, ok := bm.uint16At(keyOffset)
		if !ok {
			return nil
		}
		keyOffset += SizeKeyLen
		keysMatch := bm.compareAt(keyOffset, keyBytes)
		keyOffset += keyLen
		t, ok := bm.byteAt(keyOffset)
		if !ok {
			return nil
		}
		keyOffset += SizeValueType
		if t == TypeNil {
			if keysMatch {
				return nil
			}
		} else {
			valueOffset, ok := bm.uint32At(keyOffset)
			if !ok {
				return nil
			}
			if firstValueOffset == 0 {
				firstValueOffset = valueOffset
			}
			if keysMatch {
				return bm.valueBytesAt(valueOffset, t)
			}
			keyOffset += SizeValueOffset
		}
		if firstValueOffset > 0 && keyOffset >= firstValueOffset {
			break
		}
	}
	return nil
}

// AsMap returns a map representation of this ByteMap.
func (bm ByteMap) AsMap() map[string]interface{} {
	result := make(map[string]interface{}, 10)
	bm.IterateValues(func(key string, value interface{}) bool {
		result[key] = value
		return true
	})
	return result
}

// IterateValues iterates over the key/value pairs in this ByteMap and calls the
// given callback with each. If the callback returns false, iteration stops even
// if there remain unread values.
func (bm ByteMap) IterateValues(cb func(key string, value interface{}) bool) {
	bm.Iterate(true, false, func(key string, value interface{}, valueBytes []byte) bool {
		return cb(key, value)
	})
}

// IterateValueBytes iterates over the key/value bytes pairs in this ByteMap and
// calls the given callback with each. If the callback returns false, iteration
// stops even if there remain unread values.
func (bm ByteMap) IterateValueBytes(cb func(key string, valueBytes []byte) bool) {
	bm.Iterate(false, true, func(key string, value interface{}, valueBytes []byte) bool {
		return cb(key, valueBytes)
	})
}

// Iterate iterates over the key/value pairs in this ByteMap and calls the given
// callback with each. If the callback returns false, iteration stops even if
// there remain unread values. includeValue and includeBytes determine whether
// to include the value, the bytes or both in the callback.
func (bm ByteMap) Iterate(includeValue bool, includeBytes bool, cb func(key string, value interface{}, valueBytes []byte) bool) {
	if len(bm) == 0 {
		return
	}

	keyOffset := 0
	firstValueOffset := 0
	for {
		if keyOffset >= len(bm) {
			break
		}
		keyLen := int(enc.Uint16(bm[keyOffset:]))
		keyOffset += SizeKeyLen
		key := string(bm[keyOffset : keyOffset+keyLen])
		keyOffset += keyLen
		t := bm[keyOffset]
		keyOffset += SizeValueType
		var value interface{}
		var bytes []byte
		if t != TypeNil {
			valueOffset := int(enc.Uint32(bm[keyOffset:]))
			if firstValueOffset == 0 {
				firstValueOffset = valueOffset
			}
			if includeValue {
				value = bm.decodeValueAt(valueOffset, t)
			}
			if includeBytes {
				bytes = bm.valueBytesAt(valueOffset, t)
			}
			keyOffset += SizeValueOffset
		}
		if !cb(key, value, bytes) {
			// Stop iterating
			return
		}
		if firstValueOffset > 0 && keyOffset >= firstValueOffset {
			break
		}
	}
}

// Slice creates a new ByteMap that contains only the specified keys from the
// original.
func (bm ByteMap) Slice(includeKeys map[string]bool) ByteMap {
	result, _ := bm.doSplit(false, includeKeys)
	return result
}

// Split returns two byte maps, the first containing all of the specified keys
// and the second containing all of the other keys.
func (bm ByteMap) Split(includeKeys map[string]bool) (ByteMap, ByteMap) {
	return bm.doSplit(true, includeKeys)
}

func (bm ByteMap) doSplit(includeOmitted bool, includeKeys map[string]bool) (ByteMap, ByteMap) {
	matchedKeys := make([][]byte, 0, len(includeKeys))
	matchedValueOffsets := make([]int, 0, len(includeKeys))
	matchedValues := make([][]byte, 0, len(includeKeys))
	matchedKeysLen := 0
	matchedValuesLen := 0
	var omittedKeys [][]byte
	var omittedValueOffsets []int
	var omittedValues [][]byte
	omittedKeysLen := 0
	omittedValuesLen := 0
	if includeOmitted {
		omittedKeys = make([][]byte, 0, 10)
		omittedValueOffsets = make([]int, 0, 10)
		omittedValues = make([][]byte, 0, 10)
	}
	keyOffset := 0
	firstValueOffset := 0

	for {
		if keyOffset >= len(bm) {
			break
		}
		keyStart := keyOffset
		keyLen := int(enc.Uint16(bm[keyOffset:]))
		keyOffset += SizeKeyLen
		candidate := bm[keyOffset : keyOffset+keyLen]
		matched := includeKeys[string(candidate)]
		keyOffset += keyLen
		t := bm[keyOffset]
		keyOffset += SizeValueType
		if t != TypeNil {
			valueOffset := int(enc.Uint32(bm[keyOffset:]))
			if firstValueOffset == 0 {
				firstValueOffset = valueOffset
			}
			valueLen := bm.lengthOf(valueOffset, t)
			value := bm[valueOffset : valueOffset+valueLen]

			if matched {
				matchedKeys = append(matchedKeys, bm[keyStart:keyOffset])
				matchedValueOffsets = append(matchedValueOffsets, matchedValuesLen)
				matchedValues = append(matchedValues, value)
				matchedKeysLen += keyOffset + SizeValueOffset - keyStart
				matchedValuesLen += valueLen
			} else if includeOmitted {
				omittedKeys = append(omittedKeys, bm[keyStart:keyOffset])
				omittedValueOffsets = append(omittedValueOffsets, omittedValuesLen)
				omittedValues = append(omittedValues, value)
				omittedKeysLen += keyOffset + SizeValueOffset - keyStart
				omittedValuesLen += valueLen
			}

			keyOffset += SizeValueOffset
		}

		if keyOffset >= firstValueOffset {
			break
		}

		if !includeOmitted && len(matchedKeys) == len(includeKeys) {
			break
		}
	}

	included := buildFromSliced(matchedKeysLen, matchedValuesLen, matchedKeys, matchedValueOffsets, matchedValues)
	var omitted ByteMap
	if includeOmitted {
		omitted = buildFromSliced(omittedKeysLen, omittedValuesLen, omittedKeys, omittedValueOffsets, omittedValues)
	}
	return included, omitted
}

func buildFromSliced(keysLen int, valuesLen int, keys [][]byte, valueOffsets []int, values [][]byte) ByteMap {
	out := make(ByteMap, keysLen+valuesLen)
	offset := 0
	for i, kb := range keys {
		valueOffset := valueOffsets[i]
		copy(out[offset:], kb)
		offset += len(kb)
		if valueOffset >= 0 {
			enc.PutUint32(out[offset:], uint32(valueOffset+keysLen))
			offset += SizeValueOffset
		}
	}
	for _, vb := range values {
		copy(out[offset:], vb)
		offset += len(vb)
	}
	return out
}

func encodeValue(slice []byte, value interface{}) (byte, int) {
	switch v := value.(type) {
	case bool:
		if v {
			slice[0] = 1
		} else {
			slice[0] = 0
		}
		return TypeBool, 1
	case byte:
		slice[0] = v
		return TypeByte, 1
	case uint16:
		enc.PutUint16(slice, v)
		return TypeUInt16, 2
	case uint32:
		enc.PutUint32(slice, v)
		return TypeUInt32, 4
	case uint64:
		enc.PutUint64(slice, v)
		return TypeUInt64, 8
	case uint:
		enc.PutUint64(slice, uint64(v))
		return TypeUInt, 8
	case int8:
		slice[0] = byte(v)
		return TypeInt8, 1
	case int16:
		enc.PutUint16(slice, uint16(v))
		return TypeInt16, 2
	case int32:
		enc.PutUint32(slice, uint32(v))
		return TypeInt32, 4
	case int64:
		enc.PutUint64(slice, uint64(v))
		return TypeInt64, 8
	case int:
		enc.PutUint64(slice, uint64(v))
		return TypeInt, 8
	case []int:
		enc.PutUint16(slice, uint16(len(v)))
		for i, f := range v {
			enc.PutUint64(slice[2+i*8:], uint64(f))
		}
		return TypeInts, len(v)*8 + 2
	case float32:
		enc.PutUint32(slice, math.Float32bits(v))
		return TypeFloat32, 4
	case float64:
		enc.PutUint64(slice, math.Float64bits(v))
		return TypeFloat64, 8
	case []float64:
		enc.PutUint16(slice, uint16(len(v)))
		for i, f := range v {
			enc.PutUint64(slice[2+i*8:], math.Float64bits(f))
		}
		return TypeFloat64s, len(v)*8 + 2
	case string:
		enc.PutUint16(slice, uint16(len(v)))
		copy(slice[2:], v)
		return TypeString, len(v) + 2
	case []byte:
		enc.PutUint16(slice, uint16(len(v)))
		copy(slice[2:], v)
		return TypeBytes, len(v) + 2
	case time.Time:
		enc.PutUint64(slice, uint64(v.UnixNano()))
		return TypeTime, 8
	}
	return TypeNil, 0
}

func (bm ByteMap) decodeValueAt(offset int, t byte) interface{} {
	switch t {
	case TypeBool:
		if bm.offsetTooHigh(offset, 1) {
			return nil
		}
		return bm[offset] == 1
	case TypeByte:
		if bm.offsetTooHigh(offset, 1) {
			return nil
		}
		return bm[offset]
	case TypeUInt16:
		if bm.offsetTooHigh(offset, 2) {
			return nil
		}
		return enc.Uint16(bm[offset:])
	case TypeUInt32:
		if bm.offsetTooHigh(offset, 4) {
			return nil
		}
		return enc.Uint32(bm[offset:])
	case TypeUInt64:
		if bm.offsetTooHigh(offset, 8) {
			return nil
		}
		return enc.Uint64(bm[offset:])
	case TypeUInt:
		if bm.offsetTooHigh(offset, 8) {
			return nil
		}
		return uint(enc.Uint64(bm[offset:]))
	case TypeInt8:
		if bm.offsetTooHigh(offset, 1) {
			return nil
		}
		return int8(bm[offset])
	case TypeInt16:
		if bm.offsetTooHigh(offset, 2) {
			return nil
		}
		return int16(enc.Uint16(bm[offset:]))
	case TypeInt32:
		if bm.offsetTooHigh(offset, 4) {
			return nil
		}
		return int32(enc.Uint32(bm[offset:]))
	case TypeInt64:
		if bm.offsetTooHigh(offset, 8) {
			return nil
		}
		return int64(enc.Uint64(bm[offset:]))
	case TypeInt:
		if bm.offsetTooHigh(offset, 8) {
			return nil
		}
		return int(enc.Uint64(bm[offset:]))
	case TypeInts:
		if bm.offsetTooHigh(offset, 2) {
			return nil
		}
		l := int(enc.Uint16(bm[offset:]))
		if bm.offsetTooHigh(offset+2, l*8) {
			return nil
		}
		result := make([]int, l)
		for i := 0; i < l; i++ {
			result[i] = int(enc.Uint64(bm[offset+2+i*8:]))
		}
		return result
	case TypeFloat32:
		if bm.offsetTooHigh(offset, 4) {
			return nil
		}
		return math.Float32frombits(enc.Uint32(bm[offset:]))
	case TypeFloat64:
		if bm.offsetTooHigh(offset, 8) {
			return nil
		}
		return math.Float64frombits(enc.Uint64(bm[offset:]))
	case TypeFloat64s:
		if bm.offsetTooHigh(offset, 2) {
			return nil
		}
		l := int(enc.Uint16(bm[offset:]))
		if bm.offsetTooHigh(offset+2, l*8) {
			return nil
		}
		result := make([]float64, l)
		for i := 0; i < l; i++ {
			result[i] = math.Float64frombits(enc.Uint64(bm[offset+2+i*8:]))
		}
		return result
	case TypeString:
		if bm.offsetTooHigh(offset, 2) {
			return nil
		}
		l := int(enc.Uint16(bm[offset:]))
		if bm.offsetTooHigh(offset+2, l) {
			return nil
		}
		return string(bm[offset+2 : offset+2+l])
	case TypeBytes:
		if bm.offsetTooHigh(offset, 2) {
			return nil
		}
		l := int(enc.Uint16(bm[offset:]))
		if bm.offsetTooHigh(offset+2, l) {
			return nil
		}
		return []byte(bm[offset+2 : offset+2+l])
	case TypeTime:
		if bm.offsetTooHigh(offset, 8) {
			return nil
		}
		nanos := int64(enc.Uint64(bm[offset:]))
		second := int64(time.Second)
		return time.Unix(nanos/second, nanos%second)
	}
	return nil
}

func (bm ByteMap) valueBytesAt(offset int, t byte) []byte {
	switch t {
	case TypeBool, TypeByte, TypeInt8:
		if bm.offsetTooHigh(offset, 1) {
			return nil
		}
		return bm[offset : offset+1]
	case TypeUInt16, TypeInt16:
		if bm.offsetTooHigh(offset, 2) {
			return nil
		}
		return bm[offset : offset+2]
	case TypeUInt32, TypeInt32, TypeFloat32:
		if bm.offsetTooHigh(offset, 4) {
			return nil
		}
		return bm[offset : offset+4]
	case TypeUInt64, TypeUInt, TypeInt64, TypeInt, TypeFloat64, TypeTime:
		if bm.offsetTooHigh(offset, 8) {
			return nil
		}
		return bm[offset : offset+8]
	case TypeInts:
		if bm.offsetTooHigh(offset, 2) {
			return nil
		}
		l := int(enc.Uint16(bm[offset:]))
		if bm.offsetTooHigh(offset+2, l*8) {
			return nil
		}
		return bm[offset : offset+2+l*8]
	case TypeFloat64s:
		if bm.offsetTooHigh(offset, 2) {
			return nil
		}
		l := int(enc.Uint16(bm[offset:]))
		if bm.offsetTooHigh(offset+2, l*8) {
			return nil
		}
		return bm[offset : offset+2+l*8]
	case TypeString, TypeBytes:
		if bm.offsetTooHigh(offset, 2) {
			return nil
		}
		l := int(enc.Uint16(bm[offset:]))
		if bm.offsetTooHigh(offset+2, l) {
			return nil
		}
		return bm[offset : offset+2+l]
	}
	return nil
}

func encodedLength(value interface{}) int {
	switch v := value.(type) {
	case bool, byte, int8:
		return 1
	case uint16, int16:
		return 2
	case uint32, int32, float32:
		return 4
	case uint64, int64, uint, int, float64, time.Time:
		return 8
	case []int:
		return len(v)*8 + 2
	case []float64:
		return len(v)*8 + 2
	case string:
		return len(v) + 2
	case []byte:
		return len(v) + 2
	}
	return 0
}

func (bm ByteMap) lengthOf(valueOffset int, t byte) int {
	switch t {
	case TypeBool, TypeByte, TypeInt8:
		return 1
	case TypeUInt16, TypeInt16:
		return 2
	case TypeUInt32, TypeInt32, TypeFloat32:
		return 4
	case TypeUInt64, TypeInt64, TypeUInt, TypeInt, TypeFloat64, TypeTime:
		return 8
	case TypeInts, TypeFloat64s:
		return int(enc.Uint16(bm[valueOffset:]))*8 + 2
	case TypeString, TypeBytes:
		return int(enc.Uint16(bm[valueOffset:])) + 2
	}
	return 0
}

func (bm ByteMap) byteAt(offset int) (b byte, ok bool) {
	if bm.offsetTooHigh(offset, 1) {
		return 0, false
	}
	return bm[offset], true
}

func (bm ByteMap) uint16At(offset int) (result int, ok bool) {
	if bm.offsetTooHigh(offset, 2) {
		return 0, false
	}
	return int(enc.Uint16(bm[offset:])), true
}

func (bm ByteMap) uint32At(offset int) (result int, ok bool) {
	if bm.offsetTooHigh(offset, 4) {
		return 0, false
	}
	return int(enc.Uint32(bm[offset:])), true
}

func (bm ByteMap) compareAt(offset int, expected []byte) bool {
	lenExpected := len(expected)
	if bm.offsetTooHigh(offset, lenExpected) {
		return false
	}
	return bytes.Equal(bm[offset:offset+lenExpected], expected)
}

func (bm ByteMap) offsetTooHigh(offset int, readWidth int) bool {
	return offset+readWidth > len(bm)
}
