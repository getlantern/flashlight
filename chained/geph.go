package chained

import (
	"fmt"
	"net"
	"time"

	"github.com/getlantern/errors"
	"golang.org/x/crypto/ed25519"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/geph-official/geph2/libs/tinyss"
)

func negotiateTinySS(rawConn net.Conn, pk []byte) (cryptConn *tinyss.Socket, err error) {
	rawConn.SetDeadline(time.Now().Add(time.Second * 20))
	cryptConn, err = tinyss.Handshake(rawConn, 0 /*nextProto*/)
	if err != nil {
		err = fmt.Errorf("tinyss handshake failed: %w", err)
		rawConn.Close()
		return
	}
	// verify the actual msg
	var sssig []byte
	err = rlp.Decode(cryptConn, &sssig)
	if err != nil {
		err = fmt.Errorf("cannot decode sssig: %w", err)
		rawConn.Close()
		return
	}
	if !ed25519.Verify(pk, cryptConn.SharedSec(), sssig) {
		err = errors.New("man in the middle")
		rawConn.Close()
		return
	}
	return
}
