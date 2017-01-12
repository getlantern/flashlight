package ws

// EnvelopeType is the type of the message envelope.
type EnvelopeType struct {
	Type string `json:"type,inline"`
}

// Envelope is a struct that wraps messages and associates them with a type.
type Envelope struct {
	EnvelopeType
	Message interface{} `json:"message"`
}
