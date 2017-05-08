package service

import "testing"

var mockServiceType = Type("mockService")

type mockImpl struct {
	r *Registry
}

func (i *mockImpl) GetType() Type {
	return mockServiceType
}
func (i *mockImpl) Start() {
}
func (i *mockImpl) Stop() {
}
func (i *mockImpl) Reconfigure(r *Registry, opts ConfigOpts) {
}
func TestRegister(t *testing.T) {
}
