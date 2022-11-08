package shadowaead

import (
	"crypto/cipher"
	"errors"
	"strconv"
)

// ErrRepeatedSalt means detected a reused salt
var ErrRepeatedSalt = errors.New("repeated salt detected")

type Cipher interface {
	KeySize() int
	SaltSize() int
	Encrypter(salt []byte) (cipher.AEAD, error)
	Decrypter(salt []byte) (cipher.AEAD, error)
}

type KeySizeError int

func (e KeySizeError) Error() string {
	return "key size error: need " + strconv.Itoa(int(e)) + " bytes"
}
