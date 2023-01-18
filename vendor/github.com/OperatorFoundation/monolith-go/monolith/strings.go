package monolith

type StringType interface {
	Validateable
	Parseable
	StringFromArgs(args *Args, context *Context) (string, error)
}

type StringMessage struct {
	String string
}

func (s StringMessage) Bytes() []byte {
	return []byte(s.String)
}

type StringsPart struct {
	Items []StringType
}

type FixedStringType struct {
	String string
}

type VariableStringType struct {
	EndDelimiter byte
}
