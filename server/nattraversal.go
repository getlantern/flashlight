package server

import (
	"io"
	"net"
	"time"

	"github.com/getlantern/flashlight/log"
	//	"github.com/getlantern/go-natty/natty"
	"github.com/getlantern/waddell"
)

const (
	MaxWaddellMessageSize = 4096
)

func (server *Server) acceptNATTraversals() {
	err := server.connectToWaddell()
	if err != nil {
		log.Errorf("Unable to connect to waddell: %s", err)
		return
	}
	log.Debugf("Connected to Waddell!!! Id is: %s", server.wc.ID())

	server.receiveOffers()
}

func (server *Server) stopAcceptingNATTraversals() {
	log.Debugf("Closing waddellConn")
	server.waddellConn.Close()
	log.Debugf("Closed waddellConn")
	server.waddellConn = nil
	server.wc = nil
}

func (server *Server) connectToWaddell() (err error) {
	server.waddellConn, err = net.DialTimeout("tcp", server.waddellAddr, 20*time.Second)
	if err != nil {
		return err
	}
	server.wc, err = waddell.Connect(server.waddellConn)
	return err
}

func (server *Server) receiveOffers() {
	for {
		b := make([]byte, MaxWaddellMessageSize+waddell.WADDELL_OVERHEAD)
		wm, err := server.wc.Receive(b)
		if err != nil {
			log.Errorf("Unable to read message from waddell: %s", err)
			if err != io.EOF || err != io.ErrUnexpectedEOF {
				return
			}
			continue
		}
		log.Debugf("Received waddell message: %s", string(wm.Body))
	}
}
