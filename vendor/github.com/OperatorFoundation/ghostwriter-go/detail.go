package ghostwriter

import (
	"fmt"
	"strconv"
)

type Types int

const (
	String Types = iota
	Int
	UInt
	Float
	Data
)

type Detail interface {
	ToString() string
}

type DetailString struct {
	String string
}

type DetailInt struct {
	Int int
}

type DetailUInt struct {
	Uint uint
}

type DetailFloat struct {
	Float float32
}

type DetailData struct {
	Data []byte
}

func (detail DetailString) ToString() string {
	return detail.String
}

func (detail DetailInt) ToString() string {
	return strconv.Itoa(detail.Int)
}

func (detail DetailUInt) ToString() string {
	return strconv.FormatUint(uint64(detail.Uint), 10)
}

func (detail DetailFloat) ToString() string {
	return fmt.Sprintf("%v", detail.Float)
}

func (detail DetailData) ToString() string {
	return string(detail.Data)
}
