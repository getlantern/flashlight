// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/anacrolix/torrent (interfaces: Reader)
//
// Generated by this command:
//
//	mockgen -package=chained -destination=torrent_reader_mock_test.go github.com/anacrolix/torrent Reader
//

// Package chained is a generated GoMock package.
package chained

import (
	context "context"
	reflect "reflect"

	torrent "github.com/anacrolix/torrent"
	gomock "go.uber.org/mock/gomock"
)

// MockReader is a mock of Reader interface.
type MockReader struct {
	ctrl     *gomock.Controller
	recorder *MockReaderMockRecorder
}

// MockReaderMockRecorder is the mock recorder for MockReader.
type MockReaderMockRecorder struct {
	mock *MockReader
}

// NewMockReader creates a new mock instance.
func NewMockReader(ctrl *gomock.Controller) *MockReader {
	mock := &MockReader{ctrl: ctrl}
	mock.recorder = &MockReaderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockReader) EXPECT() *MockReaderMockRecorder {
	return m.recorder
}

// Close mocks base method.
func (m *MockReader) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockReaderMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockReader)(nil).Close))
}

// Read mocks base method.
func (m *MockReader) Read(arg0 []byte) (int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Read", arg0)
	ret0, _ := ret[0].(int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Read indicates an expected call of Read.
func (mr *MockReaderMockRecorder) Read(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Read", reflect.TypeOf((*MockReader)(nil).Read), arg0)
}

// ReadContext mocks base method.
func (m *MockReader) ReadContext(arg0 context.Context, arg1 []byte) (int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadContext", arg0, arg1)
	ret0, _ := ret[0].(int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReadContext indicates an expected call of ReadContext.
func (mr *MockReaderMockRecorder) ReadContext(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadContext", reflect.TypeOf((*MockReader)(nil).ReadContext), arg0, arg1)
}

// Seek mocks base method.
func (m *MockReader) Seek(arg0 int64, arg1 int) (int64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Seek", arg0, arg1)
	ret0, _ := ret[0].(int64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Seek indicates an expected call of Seek.
func (mr *MockReaderMockRecorder) Seek(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Seek", reflect.TypeOf((*MockReader)(nil).Seek), arg0, arg1)
}

// SetReadahead mocks base method.
func (m *MockReader) SetReadahead(arg0 int64) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetReadahead", arg0)
}

// SetReadahead indicates an expected call of SetReadahead.
func (mr *MockReaderMockRecorder) SetReadahead(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetReadahead", reflect.TypeOf((*MockReader)(nil).SetReadahead), arg0)
}

// SetReadaheadFunc mocks base method.
func (m *MockReader) SetReadaheadFunc(arg0 torrent.ReadaheadFunc) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetReadaheadFunc", arg0)
}

// SetReadaheadFunc indicates an expected call of SetReadaheadFunc.
func (mr *MockReaderMockRecorder) SetReadaheadFunc(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetReadaheadFunc", reflect.TypeOf((*MockReader)(nil).SetReadaheadFunc), arg0)
}

// SetResponsive mocks base method.
func (m *MockReader) SetResponsive() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetResponsive")
}

// SetResponsive indicates an expected call of SetResponsive.
func (mr *MockReaderMockRecorder) SetResponsive() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetResponsive", reflect.TypeOf((*MockReader)(nil).SetResponsive))
}