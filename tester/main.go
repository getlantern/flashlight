package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// flushGrace bounds the export of the spans still buffered when the tester
// stops; the pinger removes the container shortly after its deadline.
const flushGrace = 4 * time.Second

// parentContext returns the W3C trace context in TRACEPARENT, so the
// tester's spans join the trace of the run that started it, or nil when
// there is none or it is invalid.
func parentContext() context.Context {
	tp := os.Getenv("TRACEPARENT")
	if tp == "" {
		return nil
	}
	ctx := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier{"traceparent": tp})
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return nil
	}
	return ctx
}

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
	// Flashlight begins its ops on goroutines of its own, which would each
	// start a trace; with a parent set they all report under the pinger's
	// span instead.
	if parent := parentContext(); parent != nil {
		ops.SetParentContext(parent)
	}
}

func clearPreviousResult(dataDir string) error {
	for _, name := range []string{"success", "fetch.json", "connection.txt", "output.txt", "timing.txt"} {
		if err := os.Remove(filepath.Join(dataDir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func performLanternPing(urlToHit string, runId string, deviceId string, userId int64, token string, dataDir string, isSticky bool, signozKey string) error {
	if err := clearPreviousResult(dataDir); err != nil {
		return err
	}
	seconds := 20
	if value := os.Getenv("FETCH_TIMEOUT_SECONDS"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 || n > 600 {
			return fmt.Errorf("FETCH_TIMEOUT_SECONDS must be between 1 and 600")
		}
		seconds = n
	}
	golog.SetPrepender(func(writer io.Writer) {
		_, _ = writer.Write([]byte(fmt.Sprintf("%s: ", time.Now().Format("2006-01-02 15:04:05"))))
	})

	target, err := url.Parse(urlToHit)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" {
		return fmt.Errorf("target must be an HTTPS URL")
	}
	configURL := os.Getenv("CONFIG_URL")
	if !isSticky {
		expected := map[string]string{
			"staging": "https://api.staging.iantem.io/v1/config",
			"prod":    "https://api.iantem.io/v1/config",
		}[os.Getenv("ENVIRONMENT")]
		if expected == "" || configURL != expected {
			return fmt.Errorf("explicit configuration URL must match ENVIRONMENT")
		}
	}
	common.GetHTTPClient().CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || req.URL.Scheme != "https" || req.URL.Host != via[0].URL.Host {
			return fmt.Errorf("configuration redirect changes origin or exceeds limit")
		}
		return nil
	}
	settings := common.NewUserConfigData("lantern", deviceId, userId, token, nil, "en-US")
	statsTracker := stats.NewTracker()
	var onOneProxy sync.Once
	proxyReady := make(chan struct{})
	configureOtel(runId, urlToHit, signozKey)
	op := ops.Begin("tester").Set("target_url", urlToHit).Set("sticky", isSticky)
	defer op.End()
	common.LibraryVersion = "999.999.999"
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	startupCtx, cancelStartup := context.WithTimeout(runCtx, time.Duration(seconds)*time.Second)
	defer cancelStartup()
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
			"cloudconfig":    configURL,
			"globalconfig":   "https://globalconfig.flashlightproxy.com/global.yaml.gz",
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
				close(proxyReady)
			})
		}),
	)
	if err != nil {
		return err
	}
	type outcome struct {
		err                    error
		output                 string
		connectedMS, fetchedMS int64
	}
	resultCh := make(chan outcome, 1)
	complete := func(result outcome) {
		select {
		case resultCh <- result:
		default:
		}
	}
	t1 := time.Now()
	if err := os.WriteFile(filepath.Join(dataDir, "connection.txt"), []byte("connecting"), 0o600); err != nil {
		return err
	}
	go fc.Run("127.0.0.1:0", "127.0.0.1:0", func(cl *client.Client) {
		go func() {
			sa, ok := cl.Socks5Addr(min(5*time.Second, time.Duration(seconds)*time.Second))
			if !ok {
				complete(outcome{err: fmt.Errorf("failed to get socks5 address")})
				return
			}
			select {
			case <-proxyReady:
			case <-startupCtx.Done():
				complete(outcome{err: fmt.Errorf("proxy startup: %w", startupCtx.Err())})
				return
			}
			if err := startupCtx.Err(); err != nil {
				complete(outcome{err: fmt.Errorf("proxy startup: %w", err)})
				return
			}
			result := outcome{connectedMS: time.Since(t1).Milliseconds()}
			if err := os.WriteFile(filepath.Join(dataDir, "connection.txt"), []byte("connected"), 0o600); err != nil {
				complete(outcome{err: err})
				return
			}
			fetchCtx, cancelFetch := context.WithTimeout(runCtx, time.Duration(seconds+5)*time.Second)
			defer cancelFetch()
			cmd := exec.CommandContext(fetchCtx, "curl", "--proxy", fmt.Sprintf("socks5h://%s", sa), "--noproxy", "", "--silent", "--show-error",
				"--location", "--max-redirs", "10", "--proto", "=https", "--proto-redir", "=https",
				"--max-time", strconv.Itoa(seconds), "--output", dataDir+"/output.txt", "--write-out", "%{json}", urlToHit)
			output, err := cmd.Output()
			result.output, result.err, result.fetchedMS = string(output), err, time.Since(t1).Milliseconds()
			complete(result)
		}()
	}, func(err error) { complete(outcome{err: err}) })
	result := <-resultCh
	cancelRun()
	defer fc.Stop()
	runErr, output := result.err, result.output
	if output != "" {
		if err := os.WriteFile(dataDir+"/fetch.json", []byte(output), 0644); err != nil {
			return fmt.Errorf("write fetch metadata: %w", err)
		}
	}
	if err := os.WriteFile(dataDir+"/timing.txt", []byte(fmt.Sprintf(`
result: %v
run-id: %s
err: %v
started: %s
connected: %d
fetched: %d
url: %s`,
		runErr == nil, runId, runErr, t1, result.connectedMS, result.fetchedMS, urlToHit)), 0644); err != nil {
		return fmt.Errorf("write timing metadata: %w", err)
	}
	if runErr == nil {
		if err := os.WriteFile(filepath.Join(dataDir, "success"), nil, 0o600); err != nil {
			return fmt.Errorf("write success marker: %w", err)
		}
	}
	return op.FailIf(runErr)
}

// flush exports the spans still buffered, giving up after flushGrace.
func flush() {
	done := make(chan struct{})
	go func() {
		flashlightOtel.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(flushGrace):
		fmt.Fprintln(os.Stderr, "trace export did not finish in time")
	}
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
	if data != "" {
		if err := clearPreviousResult(data); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

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

	seconds, _ := strconv.Atoi(os.Getenv("FETCH_TIMEOUT_SECONDS"))
	if seconds <= 0 || seconds > 600 {
		seconds = 20
	}
	done := make(chan error, 1)
	go func() { done <- performLanternPing(targetUrl, runId, deviceId, uid, token, data, isSticky, signozKey) }()
	failed := true
	select {
	case err := <-done:
		failed = err != nil
		if failed {
			fmt.Fprintln(os.Stderr, err)
		}
	// The pinger removes the container flushGrace after this deadline.
	case <-time.After(time.Duration(2*seconds+10) * time.Second):
		fmt.Fprintln(os.Stderr, "tester deadline exceeded")
	}
	flush()
	if failed {
		os.Exit(1)
	}
}
