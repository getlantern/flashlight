package engine

const useMatomo = false

type SessionParams struct {
	IP         string
	ClientId   string
	Site       string
	Port       string
	UserAgent  string
	Hostname   string
	TrackingID string
}

type Engine interface {
	GetID() string
	GetEndpoint() string
	GetSessionValues(sa *SessionParams, site string, port string) string
}

func New(trackingID string) Engine {
	if useMatomo {
		return NewMatomo()
	}
	return NewGA(trackingID)
}
