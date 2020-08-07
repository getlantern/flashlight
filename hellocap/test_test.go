package hellocap

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"

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

	hello, err := GetBrowserHello(context.Background(), noopHostMapper("localhost"))
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

func TestDefaultBrowser(t *testing.T) {
	b, err := defaultBrowser(context.Background())
	require.NoError(t, err)

	fmt.Printf("type(browser): %T\n", b)
}

func TestHitHellocapServer(t *testing.T) {
	c := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	resp, err := c.Get("https://localhost:51134")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	fmt.Println(string(body))
}

func TestExecPathRegexp(t *testing.T) {
	for _, testCase := range []struct {
		input, expected string
	}{
		{`"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe" -- "%1"`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`},
		{`"C:\Program Files (x86)\Google\Chrome\Application\chrome.exe" -- "%1"`, `C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`},
		{`"C:\Program Files\Mozilla Firefox\firefox.exe" -osint -url "%1"`, `C:\Program Files\Mozilla Firefox\firefox.exe`},
	} {
		matches := execPathRegexp.FindStringSubmatch(testCase.input)
		if !assert.Greater(t, len(matches), 1) {
			continue
		}
		assert.Equal(t, testCase.expected, matches[1])
	}
}

const hcserverAddr = `https://localhost:52312`

func TestRunChrome(t *testing.T) {
	const pathToChrome = `C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`
	out, err := exec.Command(pathToChrome, "--headless", "--disable-gpu", hcserverAddr).CombinedOutput()
	require.NoError(t, err)
	fmt.Println("output:")
	fmt.Println(string(out))
}

func TestRunEdge(t *testing.T) {
	out, err := exec.Command("start", fmt.Sprintf("microsoft-edge:%s", hcserverAddr)).CombinedOutput()
	require.NoError(t, err)
	fmt.Println("output:")
	fmt.Println(string(out))
}

func TestRunFirefox(t *testing.T) {
	const (
		pathToFirefox = `C:\Program Files\Mozilla Firefox\firefox.exe`
	)
	// var (
	// 	runDll32 = filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "rundll32.exe")
	// )
	// fmt.Println("runDll32:", runDll32)

	// out, err := exec.Command(pathToFirefox, "-P", "test-profile", "-headless", "-osint", "-url", hcserverAddr).CombinedOutput()
	// require.NoError(t, err)
	// fmt.Println("output:")
	// fmt.Println(string(out))

	out, err := exec.Command("cmd", "/C", "start", "firefox", "-P", "default", "-headless", hcserverAddr).CombinedOutput()
	if len(out) > 0 {
		fmt.Println("output:")
		fmt.Println(string(out))
	}
	require.NoError(t, err)
}
