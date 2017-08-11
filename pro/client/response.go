package client

type Response struct {
	Status   string `json:"status"`
	Error    string `json:"error"`
	ErrorId  string `json:"errorId"`
	PubKey   string `json:"pubKey"`
	Provider string `json:"provider"`
	User     `json:",inline"`
	Plans    []Plan `json:",inline"`
}

type AutoconfResponse struct {
	Response      `json:",inline"`
	Email         string `json:"email"`
	AutoconfToken string `json:"autoconfToken"`
}

type CodeResponse struct {
	Response `json:",inline"`
	Code     string `json:"code"`
}
