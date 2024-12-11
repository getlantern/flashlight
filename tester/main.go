package main

import (
	"errors"
	"fmt"
	flashlightOtel "github.com/getlantern/flashlight/v7/otel"
	"github.com/getlantern/ops"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/golog"
	socksProxy "golang.org/x/net/proxy"

	"github.com/getlantern/flashlight/v7"
	"github.com/getlantern/flashlight/v7/client"
	"github.com/getlantern/flashlight/v7/common"
	"github.com/getlantern/flashlight/v7/stats"
)

func configureOtel(country string) {
	// Configure OpenTelemetry
	const replacementText = "UUID-GOES-HERE"
	const honeycombQueryTemplate = `https://ui.honeycomb.io/lantern-bc/environments/prod/datasets/flashlight?query=%7B%22time_range%22%3A300%2C%22granularity%22%3A15%2C%22breakdowns%22%3A%5B%5D%2C%22calculations%22%3A%5B%7B%22op%22%3A%22COUNT%22%7D%5D%2C%22filters%22%3A%5B%7B%22column%22%3A%22pinger-id%22%2C%22op%22%3A%22%3D%22%2C%22value%22%3A%22UUID-GOES-HERE%22%7D%5D%2C%22filter_combination%22%3A%22AND%22%2C%22orders%22%3A%5B%5D%2C%22havings%22%3A%5B%5D%2C%22trace_joins%22%3A%5B%5D%2C%22limit%22%3A100%7D`
	runId := uuid.NewString()
	fmt.Printf("performing lantern ping: url=%s\n", country)
	fmt.Printf("lookup traces on Honeycomb with pinger-id: %s, link: %s\n", runId, strings.ReplaceAll(honeycombQueryTemplate, replacementText, runId))
	flashlightOtel.ConfigureOnce(&flashlightOtel.Config{
		Endpoint: "api.honeycomb.io:443",
		Headers: map[string]string{
			"x-honeycomb-team": "vuWkzaeefr2OcL1SfowtuG",
		},
	}, "pinger")
	ops.SetGlobal("pinger-id", runId)
}

func performLanternPing(urlToHit string, runId string, deviceId string, userId int64, token string, outputDir string) error {
	golog.SetPrepender(func(writer io.Writer) {
		_, _ = writer.Write([]byte(fmt.Sprintf("%s: ", time.Now().Format("2006-01-02 15:04:05"))))
	})

	settings := common.NewUserConfigData("lantern", deviceId, userId, token, nil, "en-US")
	statsTracker := stats.NewTracker()
	var onOneProxy sync.Once
	proxyReady := make(chan struct{})
	configureOtel(urlToHit)
	common.LibraryVersion = "999.999.999"
	fc, err := flashlight.New(
		"pinger",
		"999.999.999",
		"10-10-2024",
		outputDir,
		false,
		func() bool { return false },
		func() bool { return false },
		func() bool { return false },
		func() bool { return false },
		map[string]interface{}{
			"readableconfig": true,
			//"stickyconfig":   true,
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

			flashlightSocksAddress, _ := url.Parse("socks5://" + sa.(string))
			t2 = time.Now()
			fmt.Printf("lantern started correctly. urlToHit: %s\n", urlToHit)

			sdialer, err := socksProxy.FromURL(flashlightSocksAddress, socksProxy.Direct)
			if err != nil {
				fmt.Printf("failed to create socks dialer: %v\n", err)
				resultCh <- err
				return
			}

			httpClient := http.Client{Transport: &http.Transport{Dial: sdialer.Dial}}
			resp, err := httpClient.Get(urlToHit)
			if err != nil {
				fmt.Printf("failed to hit url: %v\n", err)
				resultCh <- err
				return
			}
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
				fmt.Printf("unexpected status code %d\n", resp.StatusCode)
				resultCh <- fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				return
			}
			body := make([]byte, 1024)
			n, err := resp.Body.Read(body)
			if err != nil && !errors.Is(err, io.EOF) {
				fmt.Printf("failed to read body: %v\n", err)
				resultCh <- err
				return
			}
			fmt.Printf("got body '%s'", string(body[:n]))
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
	}

	return os.WriteFile(outputDir+"/timing.txt", []byte(fmt.Sprintf(`
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
	output := os.Getenv("OUTPUT")

	if deviceId == "" || userId == "" || token == "" || runId == "" || targetUrl == "" || output == "" {
		fmt.Println("missing required environment variable(s)")
		fmt.Println("Required environment variables: DEVICE_ID, USER_ID, TOKEN, RUN_ID, TARGET_URL, OUTPUT")
		os.Exit(1)
	}

	uid, err := strconv.ParseInt(userId, 10, 64)
	if err != nil {
		fmt.Println("failed to parse USER_ID")
		os.Exit(1)
	}

	if performLanternPing(targetUrl, runId, deviceId, uid, token, output) != nil {
		fmt.Println("failed to perform lantern ping")
		os.Exit(1)
	}
}
