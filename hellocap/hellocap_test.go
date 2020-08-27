package hellocap

import (
	"crypto/tls"
	"errors"
	"net/http"
	"testing"

	"github.com/getlantern/tlsutil"
	"github.com/stretchr/testify/require"
)

func TestCapturingServer(t *testing.T) {
	callbackInvoked := make(chan struct{})
	s, err := NewServer(func(hello []byte, err error) {
		close(callbackInvoked)
		require.NoError(t, err)
		// Testing with tlsutil.ValidateClientHello is a bit circular, but we don't have another
		// easy way to validate the content of hello.
		_, err = tlsutil.ValidateClientHello(hello)
		require.NoError(t, err)
	})
	require.NoError(t, err)
	defer s.Close()

	go func() {
		err := s.Start()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case <-callbackInvoked:
			default:
				t.Log("error returned by listenAndServeTLS:", err)
				t.Fail()
			}
		}
	}()

	conn, err := tls.Dial("tcp", s.Addr().String(), &tls.Config{InsecureSkipVerify: true})
	require.NoError(t, err)
	require.NoError(t, conn.Handshake())

	<-callbackInvoked
}
