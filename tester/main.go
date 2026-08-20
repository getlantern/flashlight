package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/getlantern/flashlight/v7"
	"github.com/getlantern/flashlight/v7/client"
	"github.com/getlantern/flashlight/v7/common"
	flashlightOtel "github.com/getlantern/flashlight/v7/otel"
	"github.com/getlantern/flashlight/v7/stats"
	"github.com/getlantern/golog"
	"github.com/getlantern/ops"
)

func configureOtel(runId, country, signozKey string) {
	fmt.Printf("performing lantern ping: url=%s\n", country)
	fmt.Printf("lookup traces on SigNoz with pinger-id=%s\n  https://lantern.us.signoz.cloud/traces-explorer\n", runId)
	ops.SetGlobal("pinger-id", runId)
	flashlightOtel.ConfigureOnce(&flashlightOtel.Config{
		Endpoint: "ingest.us.signoz.cloud:443",
		Headers: map[string]string{
			"signoz-ingestion-key": signozKey,
		},
	}, "pinger")
}

func performLanternPing(urlToHit string, runId string, deviceId string, userId int64, token string, dataDir string, isSticky bool, signozKey string) error {
	golog.SetPrepender(func(writer io.Writer) {
		_, _ = writer.Write([]byte(fmt.Sprintf("%s: ", time.Now().Format("2006-01-02 15:04:05"))))
	})

	settings := common.NewUserConfigData("lantern", deviceId, userId, token, nil, "en-US")
	statsTracker := stats.NewTracker()
	var onOneProxy sync.Once
	proxyReady := make(chan struct{})
	configureOtel(runId, urlToHit, signozKey)
	common.LibraryVersion = "999.999.999"
	fc, err := flashlight.New(
		"pinger",
		"999.999.999",
		"10-10-2024",
		dataDir,
		false,
		func() bool { return false },
		func() bool { return false },
		func() bool { return false },
		func() bool { return false },
		map[string]interface{}{
			"readableconfig": true,
			"stickyconfig":   isSticky,
		},
		settings,
		statsTracker,
		func() bool { return false },
		func() string { return "en-US" },
		func(host string) (string, error) {
			return host, nil
		},
		func(category, action, label string) {

		},
		flashlight.WithOnDialError(func(err error, v bool) {
			fmt.Printf("failed to dial %v %v\n", err, v)
		}),
		flashlight.WithOnSucceedingProxy(func() {
			onOneProxy.Do(func() {
				fmt.Printf("succeeding proxy\n")
				proxyReady <- struct{}{}
			})
		}),
	)
	if err != nil {
		return err
	}
	resultCh := make(chan error)
	t1 := time.Now()
	var t2, t3 time.Time
	output := ""
	go fc.Run("127.0.0.1:0", "127.0.0.1:0", func(cl *client.Client) {
		go func() {
			sa, ok := cl.Socks5Addr(5 * time.Second)
			if !ok {
				resultCh <- fmt.Errorf("failed to get socks5 address")
				return
			}
			select {
			case <-proxyReady:
				break
			}

			t2 = time.Now()
			flashlightProxy := fmt.Sprintf("socks5://%s", sa)
			fmt.Printf("lantern started correctly. urlToHit: %s flashlight proxy: %s\n", urlToHit, flashlightProxy)

			cmd := exec.Command("curl", "-x", flashlightProxy, "-s", urlToHit)

			// Run the command and capture the output
			outputB, err := cmd.Output()
			if err != nil {
				fmt.Println("Error executing command:", err)
				resultCh <- err
				return
			}

			output = string(outputB)
			t3 = time.Now()
			resultCh <- nil
		}()
	}, func(err error) {
		resultCh <- err
	})

	var runErr error
	select {
	case err := <-resultCh:
		runErr = err
		break
	}
	defer fc.Stop()

	if runErr == nil {
		fmt.Println("lantern ping completed successfully")
		// create a marker file that will be used by the pinger to determine success
		_ = os.WriteFile(dataDir+"/success", []byte(""), 0644)
	}

	_ = os.WriteFile(dataDir+"/output.txt", []byte(output), 0644)
	return os.WriteFile(dataDir+"/timing.txt", []byte(fmt.Sprintf(`
result: %v
run-id: %s
err: %v
started: %s
connected: %d
fetched: %d
url: %s`,
		runErr == nil, runId, runErr, t1, int32(t2.Sub(t1).Milliseconds()), int32(t3.Sub(t1).Milliseconds()), urlToHit)), 0644)
}

func main() {
	deviceId := os.Getenv("DEVICE_ID")
	userId := os.Getenv("USER_ID")
	token := os.Getenv("TOKEN")
	runId := os.Getenv("RUN_ID")
	targetUrl := os.Getenv("TARGET_URL")
	data := os.Getenv("DATA")
	signozKey := os.Getenv("SIGNOZ_INGESTION_KEY")
	isSticky := os.Getenv("STICKY") == "true"

	if deviceId == "" || userId == "" || token == "" || runId == "" || targetUrl == "" || data == "" || signozKey == "" {
		fmt.Println("missing required environment variable(s)")
		fmt.Println("Required environment variables: DEVICE_ID, USER_ID, TOKEN, RUN_ID, TARGET_URL, DATA, SIGNOZ_INGESTION_KEY")
		os.Exit(1)
	}

	uid, err := strconv.ParseInt(userId, 10, 64)
	if err != nil {
		fmt.Println("failed to parse USER_ID")
		os.Exit(1)
	}

	if performLanternPing(targetUrl, runId, deviceId, uid, token, data, isSticky, signozKey) != nil {
		fmt.Println("failed to perform lantern ping")
		os.Exit(1)
	}
}
