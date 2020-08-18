package hellocap

import (
	"crypto/tls"
	"fmt"
	"testing"

	"github.com/getlantern/tlsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapturingServer(t *testing.T) {
	callbackInvoked := make(chan struct{})
	s, err := newCapturingServer(func(hello []byte, err error) {
		fmt.Println("callback invoked")
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
		assert.NoError(t, s.listenAndServeTLS())
	}()

	conn, err := tls.Dial("tcp", s.address(), &tls.Config{InsecureSkipVerify: true})
	require.NoError(t, err)
	require.NoError(t, conn.Handshake())

	<-callbackInvoked
}
