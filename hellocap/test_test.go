package hellocap

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/mitchellh/go-ps"

	"github.com/getlantern/tlsutil"
	"github.com/stretchr/testify/require"
)

const (
	hostsFile   = "/etc/hosts"
	hfmPrelude  = "# Added by getlantern/flashlight/hellocap.hostsFileManager"
	hfmPostlude = "# End of section"
)

// TODO: delete or figure out a way to make this a reliable, useful test
func TestHello(t *testing.T) {
	// It's currently important for the test to have this builtin timeout for Firefox cleanup.
	const timeout = 5 * time.Second

	fmt.Println("starting test")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	fmt.Printf("[%v] calling func\n", start)
	hello, err := GetDefaultBrowserHello(ctx)
	end := time.Now()
	fmt.Printf("[%v] func returned\n", end)
	require.NoError(t, err)

	fmt.Println("len(hello):", len(hello))
	fmt.Println("took", end.Sub(start))

	_, err = tlsutil.ValidateClientHello(hello)
	require.NoError(t, err)
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
	fmt.Println("my PID:", os.Getpid())
	fmt.Println()

	pTree, err := processTree(os.Getpid(), nil)
	require.NoError(t, err)
	fmt.Println("process tree before command:")
	printTree(*pTree, 0)
	fmt.Println()

	cmd := exec.Command("cmd", "/C", "start", "firefox", "-P", "default", "-headless", hcserverAddr)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	require.NoError(t, cmd.Run())
	cmdPID := cmd.Process.Pid
	fmt.Println("cmd PID:", cmdPID)
	fmt.Println()

	time.Sleep(2 * time.Second)

	pTree, err = processTree(os.Getpid(), nil)
	require.NoError(t, err)
	fmt.Println("\nprocess tree after command:")
	printTree(*pTree, 0)

	allProcs, err := ps.Processes()
	require.NoError(t, err)

	for _, p := range allProcs {
		if p.PPid() != cmdPID {
			continue
		}
		fmt.Println("\nfound child; executable = ", p.Executable())
		pTree, err = ptHelper(p.Pid(), allProcs)
		require.NoError(t, err)
		fmt.Println("process tree:")
		printTree(*pTree, 0)
	}

}

// Unix only.
func TestProcessTree(t *testing.T) {
	const script = `#! /bin/bash

for i in $(seq 1 10)
do
	sleep 120 & sleep_pid=$!
	echo "process sleeping with PID $sleep_pid"
done

sleep 120`

	tempFile, err := ioutil.TempFile("", "test-proc-tree")
	require.NoError(t, err)
	require.NoError(t, os.Chmod(tempFile.Name(), 0744))
	require.NoError(t, ioutil.WriteFile(tempFile.Name(), []byte(script), 0744))

	cmd := exec.Command(tempFile.Name())
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	require.NoError(t, cmd.Start())

	time.Sleep(time.Second)
	root, err := processTree(os.Getpid(), nil)
	require.NoError(t, err)
	fmt.Println()
	fmt.Println("current tree:")
	printTree(*root, 0)

	require.Equal(t, 1, len(root.children))
	fmt.Println()
	fmt.Println("killing tree rooted at", root.children[0].Executable())
	fmt.Println()
	require.NoError(t, root.children[0].kill())

	time.Sleep(5 * time.Second)
	root, err = processTree(os.Getpid(), nil)
	require.NoError(t, err)
	fmt.Println("new tree:")
	printTree(*root, 0)
}

func printTree(root process, level int) {
	for i := 0; i < level; i++ {
		fmt.Print("\t")
	}
	fmt.Println(root.Executable())
	for _, child := range root.children {
		printTree(child, level+1)
	}
}
