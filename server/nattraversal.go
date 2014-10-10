package server

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"

	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/go-natty/natty"
	"github.com/getlantern/waddell"
)

const (
	MaxWaddellMessageSize = 4096
)

type peer struct {
	id              waddell.PeerId
	traversals      map[uint32]*natty.Traversal
	traversalsMutex sync.Mutex
}

var (
	endianness = binary.LittleEndian
	peers      map[waddell.PeerId]*peer
	peersMutex sync.Mutex
	debugOut   io.Writer
)

const (
	MAX_MESSAGE_SIZE = 4096
	READY            = "READY"
	TIMEOUT          = 15 * time.Second
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
	b := make([]byte, MaxWaddellMessageSize+waddell.WADDELL_OVERHEAD)
	for {
		wm, err := server.wc.Receive(b)
		if err != nil {
			log.Errorf("Unable to read message from waddell: %s", err)
			if err != io.EOF || err != io.ErrUnexpectedEOF {
				return
			}
			continue
		}
		msg := message(wm.Body)
		log.Debugf("Peer ID is %s", wm.From.String())
		log.Debugf("Received waddell message: %s", msg)
	}
}

func (server *Server) answer(wm *waddell.Message) {
	peersMutex.Lock()
	defer peersMutex.Unlock()
	p := peers[wm.From]
	if p == nil {
		p = &peer{
			id:         wm.From,
			traversals: make(map[uint32]*natty.Traversal),
		}
		peers[wm.From] = p
	}
	p.answer(server, wm)
}

func idToBytes(id uint32) []byte {
	b := make([]byte, 4)
	endianness.PutUint32(b[:4], id)
	return b
}

func (p *peer) answer(server *Server, wm *waddell.Message) {
	p.traversalsMutex.Lock()
	defer p.traversalsMutex.Unlock()
	msg := message(wm.Body)
	traversalId := msg.getTraversalId()
	t := p.traversals[traversalId]
	if t == nil {
		log.Debugf("Answering traversal: %d", traversalId)
		// Set up a new Natty traversal
		t = natty.Answer(debugOut)
		go func() {
			// Send
			for {
				msgOut, done := t.NextMsgOut()
				if done {
					return
				}
				log.Debugf("Sending %s", msgOut)
				server.wc.SendPieces(p.id, idToBytes(traversalId), []byte(msgOut))
			}
		}()

		go func() {
			// Receive
			defer func() {
				p.traversalsMutex.Lock()
				defer p.traversalsMutex.Unlock()
				delete(p.traversals, traversalId)
			}()

			ft, err := t.FiveTupleTimeout(TIMEOUT)
			if err != nil {
				log.Debugf("Unable to answer traversal %d: %s", traversalId, err)
				return
			}

			log.Debugf("Got five tuple: %s", ft)
			//go readUDP(p.id, traversalId, ft)
		}()
		p.traversals[traversalId] = t
	}
	log.Debugf("Received for traversal %d: %s", traversalId, msg.getData())
	t.MsgIn(string(msg.getData()))
}
