package prefix

type nonePrefix struct{}

func NewNonePrefix() Prefix {
	return nonePrefix{}
}

func (p nonePrefix) Make() ([]byte, error) {
	return []byte{}, nil
}
