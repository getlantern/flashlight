package natty

import (
	"encoding/binary"
	"io"
	"math/rand"
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

const (
	MAX_MESSAGE_SIZE = 4096
	READY            = "READY"
	TIMEOUT          = 15 * time.Second
)

type PeerIds map[string]*PeerConn
type Peers map[waddell.PeerId]*Peer

type Peer struct {
	id              waddell.PeerId
	traversals      map[uint32]*natty.Traversal
	traversalsMutex sync.Mutex
}

type PeerConn struct {
	Id string
}

type message []byte

func (msg message) setTraversalId(id uint32) {
	endianness.PutUint32(msg[:4], id)
}

func (msg message) getTraversalId() uint32 {
	return endianness.Uint32(msg[:4])
}

func (msg message) getData() []byte {
	return msg[4:]
}

var (
	endianness  = binary.LittleEndian
	activePeers map[string]bool
	peers       Peers
	peersMutex  sync.Mutex
	debugOut    io.Writer
	WaddellConn net.Conn
	WaddellAddr string
	Wc          *waddell.Client
	serverReady = make(chan bool, 10)
)

func init() {
	activePeers = make(map[string]bool)
	peers = make(map[waddell.PeerId]*Peer)
	//debugOut = os.Stderr
}

func idToBytes(id uint32) []byte {
	b := make([]byte, 4)
	endianness.PutUint32(b[:4], id)
	return b
}

func ConnectToWaddell(waddellAddr string) (err error) {

	WaddellAddr = waddellAddr
	WaddellConn, err = net.DialTimeout("tcp", waddellAddr, 20*time.Second)
	if err != nil {
		return err
	}
	Wc, err = waddell.Connect(WaddellConn)
	if err != nil {
		log.Errorf("Unable to connect to waddell: %s", err)
	} else {
		log.Debugf("Connected to Waddell!!! Id is: %s", Wc.ID())
	}
	return err
}

func UpdateWaddellConn(waddellAddr, kind string,
	peers *[]PeerConn) (err error) {
	if WaddellAddr == "" || WaddellAddr != waddellAddr {
		if WaddellConn != nil {
			log.Debugf("Closing old waddell connection")
			CloseWaddellConn()
		}
		log.Debugf("New waddell addr is %s", waddellAddr)

		if waddellAddr != "" {
			WaddellAddr = waddellAddr
			err = ConnectToWaddell(waddellAddr)
			if err != nil {
				return
			}
			if kind == "server" {
				go ReceiveOffers()
			} else {
				log.Debugf("Checking peers list!!")
				CheckPeersList(peers)
			}
		}
	}
	return
}

func CheckPeersList(peers *[]PeerConn) {
	for _, peer := range *peers {
		if activePeers[peer.Id] {
			continue
		}
		activePeers[peer.Id] = true
		sendOffer(peer.Id)
	}
}

func sendMessages(t *natty.Traversal, peerId waddell.PeerId,
	traversalId uint32) {
	for {
		msgOut, done := t.NextMsgOut()
		if done {
			return
		}
		log.Debugf("Sending %s", msgOut)
		Wc.SendPieces(peerId, idToBytes(traversalId), []byte(msgOut))
	}
}

func receiveMessages(t *natty.Traversal, traversalId uint32) {
	b := make([]byte, MAX_MESSAGE_SIZE+waddell.WADDELL_OVERHEAD)
	for {
		wm, err := Wc.Receive(b)
		if err != nil {
			log.Fatalf("Unable to read message from waddell: %s", err)
		}
		msg := message(wm.Body)
		if msg.getTraversalId() != traversalId {
			log.Debugf("Got message for unknown traversal %d, skipping", msg.getTraversalId())
			continue
		}
		log.Debugf("Received: %s", msg.getData())
		msgString := string(msg.getData())
		if READY == msgString {
			// Server's ready!
			serverReady <- true
		} else {
			t.MsgIn(msgString)
		}
	}
}

func sendOffer(id string) {
	log.Debugf("Sending offer to peer %s", id)
	traversalId := uint32(rand.Int31())
	log.Debugf("Starting traversal: %d", traversalId)

	t := natty.Offer(debugOut)

	peerId, err := waddell.PeerIdFromString(id)
	if err != nil {
		log.Fatalf("Unable to parse PeerID for server %s: %s", id, err)
	}

	go sendMessages(t, peerId, traversalId)
	go receiveMessages(t, traversalId)

	ft, err := t.FiveTupleTimeout(TIMEOUT)
	if err != nil {
		log.Fatalf("Unable to offer: %s", err)
	}
	log.Debugf("Got five tuple: %s", ft)

}

func ReceiveOffers() {
	for {
		b := make([]byte, MaxWaddellMessageSize+waddell.WADDELL_OVERHEAD)
		wm, err := Wc.Receive(b)
		if err != nil {
			log.Errorf("Unable to read message from waddell: %s", err)
			if err != io.EOF || err != io.ErrUnexpectedEOF {
				return
			}
			continue
		}
		msg := []byte(wm.Body)
		log.Debugf("Peer ID is %s", wm.From.String())
		log.Debugf("Received waddell message: %s", msg[4:])
		answer(wm)
	}
}

func CloseWaddellConn() {
	log.Debugf("Closing WaddellConn")
	WaddellConn.Close()
	log.Debugf("Closed WaddellConn")
	WaddellConn = nil
	Wc = nil
}

func answer(wm *waddell.Message) {
	peersMutex.Lock()
	defer peersMutex.Unlock()
	p := peers[wm.From]
	if p == nil {
		p = &Peer{
			id:         wm.From,
			traversals: make(map[uint32]*natty.Traversal),
		}
		peers[wm.From] = p
	}
	p.answer(wm)
}

func (p *Peer) answer(wm *waddell.Message) {
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
				Wc.SendPieces(p.id, idToBytes(traversalId), []byte(msgOut))
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
