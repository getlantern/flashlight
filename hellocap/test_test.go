package hellocap

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"testing"

	"github.com/getlantern/tlsutil"
	"github.com/stretchr/testify/require"
)

// TODO: this is hacky...
type domainAddress string

func (a *domainAddress) Domain() string             { return string(*a) }
func (a *domainAddress) MapTo(address string) error { *a = domainAddress(address); return nil }
func (a *domainAddress) Clear() error               { return nil }

const (
	hostsFile   = "/etc/hosts"
	hfmPrelude  = "# Added by getlantern/flashlight/hellocap.hostsFileManager"
	hfmPostlude = "# End of section"
)

type hostsFileMapper struct {
	domain string
}

func (m hostsFileMapper) Domain() string {
	return m.domain
}

func (m hostsFileMapper) MapTo(address string) error {
	f, err := os.OpenFile(hostsFile, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("unable to open hosts file: %w", err)
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%s\n%s %s\n%s\n", hfmPrelude, address, m.domain, hfmPostlude)
	if err != nil {
		return fmt.Errorf("failed to write to hosts file: %w", err)
	}
	return nil
}

func (m hostsFileMapper) Clear() error {
	f, err := os.OpenFile(hostsFile, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("unable to open hosts file: %w", err)
	}
	defer f.Close()

	tmpFile, err := ioutil.TempFile("", "flashlight.hellocap.hostsFileManage")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	s := bufio.NewScanner(f)
	copying := true
	for s.Scan() {
		if s.Text() == hfmPrelude {
			copying = false
		}
		if copying {
			line := make([]byte, len(s.Bytes())+1)
			copy(line, s.Bytes())
			line[len(line)-1] = '\n'
			if _, err := tmpFile.Write(line); err != nil {
				return fmt.Errorf("failed to write line to tmp file: %w", err)
			}
		}
		if !copying && s.Text() == hfmPostlude {
			copying = true
		}
	}

	if err := os.Rename(tmpFile.Name(), hostsFile); err != nil {
		return fmt.Errorf("failed to overwrite hosts file: %w", err)
	}
	return nil
}

type noopHostMapper string

func (nhm noopHostMapper) Domain() string       { return string(nhm) }
func (nhm noopHostMapper) MapTo(_ string) error { return nil }
func (nhm noopHostMapper) Clear() error         { return nil }

// TODO: delete or figure out a way to make this a reliable, useful test
func TestHello(t *testing.T) {
	fmt.Println("starting test")

	hello, err := GetBrowserHello(context.Background(), noopHostMapper("i.scdn.co"))
	require.NoError(t, err)

	fmt.Println("len(hello):", len(hello))

	_, err = tlsutil.ValidateClientHello(hello)
	require.NoError(t, err)
}

func TestHFMMapTo(t *testing.T) {
	const domain, addr = "wikipedia.org", "127.0.0.1"
	require.NoError(t, hostsFileMapper{domain}.MapTo(addr))
}

func TestHFMClear(t *testing.T) {
	require.NoError(t, hostsFileMapper{}.Clear())
}

func TestTest(t *testing.T) {
	l, err := net.Listen("tcp", "")
	require.NoError(t, err)
	defer l.Close()

	fmt.Println("addr:", l.Addr().String())
	host, _, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)
	fmt.Println("host:", host)
}
