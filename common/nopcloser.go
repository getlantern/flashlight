package common

type NopCloser struct{}

func (NopCloser) Close() {}
