// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/getlantern/flashlight/v7/chained (interfaces: WASMDownloader)
//
// Generated by this command:
//
//	mockgen -destination=mocks.go -package=chained . WASMDownloader
//

// Package chained is a generated GoMock package.
package chained

import (
	context "context"
	io "io"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockWASMDownloader is a mock of WASMDownloader interface.
type MockWASMDownloader struct {
	ctrl     *gomock.Controller
	recorder *MockWASMDownloaderMockRecorder
}

// MockWASMDownloaderMockRecorder is the mock recorder for MockWASMDownloader.
type MockWASMDownloaderMockRecorder struct {
	mock *MockWASMDownloader
}

// NewMockWASMDownloader creates a new mock instance.
func NewMockWASMDownloader(ctrl *gomock.Controller) *MockWASMDownloader {
	mock := &MockWASMDownloader{ctrl: ctrl}
	mock.recorder = &MockWASMDownloaderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockWASMDownloader) EXPECT() *MockWASMDownloaderMockRecorder {
	return m.recorder
}

// DownloadWASM mocks base method.
func (m *MockWASMDownloader) DownloadWASM(arg0 context.Context, arg1 io.Writer) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DownloadWASM", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// DownloadWASM indicates an expected call of DownloadWASM.
func (mr *MockWASMDownloaderMockRecorder) DownloadWASM(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DownloadWASM", reflect.TypeOf((*MockWASMDownloader)(nil).DownloadWASM), arg0, arg1)
}
