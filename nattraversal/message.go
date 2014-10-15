package nattraversal

type Message []byte

func (msg Message) setTraversalId(id uint32) {
	endianness.PutUint32(msg[:4], id)
}

func (msg Message) getTraversalId() uint32 {
	return endianness.Uint32(msg[:4])
}

func (msg Message) getData() []byte {
	return msg[4:]
}
