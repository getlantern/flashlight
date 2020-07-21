package chained

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/getlantern/errors"
	"golang.org/x/crypto/ed25519"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/geph-official/geph2/libs/tinyss"
)

func negotiateTinySS(greeting *[2][]byte, rawConn net.Conn, pk []byte, nextProto byte) (cryptConn *tinyss.Socket, err error) {
	rawConn.SetDeadline(time.Now().Add(time.Second * 20))
	cryptConn, err = tinyss.Handshake(rawConn, nextProto)
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
	if greeting != nil {
		// send the greeting
		rlp.Encode(cryptConn, greeting)
		// wait for the reply
		var reply string
		err = rlp.Decode(cryptConn, &reply)
		if err != nil {
			err = fmt.Errorf("cannot decode reply: %w", err)
			rawConn.Close()
			return
		}
		if reply != "OK" {
			err = errors.New("authentication failed")
			rawConn.Close()
			log.Debugf("authentication failed", reply)
			os.Exit(11)
		}
	}
	return
}
