package config

import (
	"compress/gzip"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/getlantern/rot13"
	"github.com/getlantern/yaml"

	"github.com/getlantern/flashlight/chained"
)

// withTempDir creates a temporary directory, executes the given function and
// then cleans up the temporary directory. inTempDir allows modifying filenames
// to put them in the temporary directory.
func withTempDir(t *testing.T, fn func(inTempDir func(file string) string)) {
	tmpDir, err := ioutil.TempDir("", "test")
	abortOnError(t, err)
	defer os.RemoveAll(tmpDir)
	fn(func(file string) string {
		return filepath.Join(tmpDir, file)
	})
}

// writeObfuscatedConfig serializes the given config onto disk as YAML with
// ROT13 encoding.
func writeObfuscatedConfig(t *testing.T, config interface{}, obfuscatedFilename string) {
	log.Debugf("Writing obfuscated config to %v", obfuscatedFilename)

	bytes, err := yaml.Marshal(config)
	abortOnError(t, err)

	// create new obfuscated file
	outfile, err := os.OpenFile(obfuscatedFilename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	abortOnError(t, err)
	defer outfile.Close()

	// write ROT13-encoded config to obfuscated file
	out := rot13.NewWriter(outfile)
	_, err = out.Write(bytes)
	abortOnError(t, err)
}

func startConfigServer(t *testing.T, config interface{}) (u string, reqCount func() int64) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Unable to listen: %s", err)
	}

	requests := int64(0)
	hs := &http.Server{
		Handler: http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			atomic.AddInt64(&requests, 1)
			bytes, _ := yaml.Marshal(config)
			w := gzip.NewWriter(resp)
			defer w.Close()
			w.Write(bytes)
		}),
	}
	go func() {
		if err = hs.Serve(l); err != nil {
			t.Errorf("Unable to serve: %v", err)
		}
	}()

	port := l.Addr().(*net.TCPAddr).Port
	url := "http://localhost:" + strconv.Itoa(port)
	return url, func() int64 {
		return atomic.LoadInt64(&requests)
	}
}

func newGlobalConfig(t *testing.T) *Global {
	global := &Global{}
	err := yaml.Unmarshal([]byte(globalYamlTemplate), global)
	abortOnError(t, err)
	return global
}

func newProxiesConfig(t *testing.T) map[string]*chained.ChainedServerInfo {
	proxies := make(map[string]*chained.ChainedServerInfo)
	err := yaml.Unmarshal([]byte(proxiesYamlTemplate), proxies)
	abortOnError(t, err)
	return proxies
}

// abortOnError aborts the current test if there's an error
func abortOnError(t *testing.T, err error) {
	if err != nil {
		abortOnError(t, err)
	}
}

// Certain tests fetch global config from a remote server and store it at
// `global.yaml`.  Other tests rely on `global.yaml` matching the
// `fetched-global.yaml` fixture.  For tests that fetch config remotely, we must
// delete the config file once the test has completed to avoid causing other
// these other tests to fail in the event that the remote config differs from
// the fixture.
func deleteGlobalConfig() {
	os.Remove("global.yaml")
}

const globalYamlTemplate = `
version: 0
cloudconfigca: ""
autoupdateca: ""
updateserverurl: ""
bordareportinterval: 0s
bordasamplepercentage: 0
pingsamplepercentage: 0
reportissueemail: ""
client:
  dumpheaders: false
  masqueradesets:
    cloudflare: []
    cloudfront:
    - domain: ad1.awsstatic.com
      ipaddress: 54.192.34.80
adsettings: null
proxiedsites:
  delta:
    additions: []
    deletions: []
  cloud:
  - 0000a-fast-proxy.de
trustedcas:
- commonname: VeriSign Class 3 Public Primary Certification Authority - G5
  cert: "-----BEGIN CERTIFICATE-----\nMIIE0zCCA7ugAwIBAgIQGNrRniZ96LtKIVjNzGs7SjANBgkqhkiG9w0BAQUFADCB\nyjELMAkGA1UEBhMCVVMxFzAVBgNVBAoTDlZlcmlTaWduLCBJbmMuMR8wHQYDVQQL\nExZWZXJpU2lnbiBUcnVzdCBOZXR3b3JrMTowOAYDVQQLEzEoYykgMjAwNiBWZXJp\nU2lnbiwgSW5jLiAtIEZvciBhdXRob3JpemVkIHVzZSBvbmx5MUUwQwYDVQQDEzxW\nZXJpU2lnbiBDbGFzcyAzIFB1YmxpYyBQcmltYXJ5IENlcnRpZmljYXRpb24gQXV0\naG9yaXR5IC0gRzUwHhcNMDYxMTA4MDAwMDAwWhcNMzYwNzE2MjM1OTU5WjCByjEL\nMAkGA1UEBhMCVVMxFzAVBgNVBAoTDlZlcmlTaWduLCBJbmMuMR8wHQYDVQQLExZW\nZXJpU2lnbiBUcnVzdCBOZXR3b3JrMTowOAYDVQQLEzEoYykgMjAwNiBWZXJpU2ln\nbiwgSW5jLiAtIEZvciBhdXRob3JpemVkIHVzZSBvbmx5MUUwQwYDVQQDEzxWZXJp\nU2lnbiBDbGFzcyAzIFB1YmxpYyBQcmltYXJ5IENlcnRpZmljYXRpb24gQXV0aG9y\naXR5IC0gRzUwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQCvJAgIKXo1\nnmAMqudLO07cfLw8RRy7K+D+KQL5VwijZIUVJ/XxrcgxiV0i6CqqpkKzj/i5Vbex\nt0uz/o9+B1fs70PbZmIVYc9gDaTY3vjgw2IIPVQT60nKWVSFJuUrjxuf6/WhkcIz\nSdhDY2pSS9KP6HBRTdGJaXvHcPaz3BJ023tdS1bTlr8Vd6Gw9KIl8q8ckmcY5fQG\nBO+QueQA5N06tRn/Arr0PO7gi+s3i+z016zy9vA9r911kTMZHRxAy3QkGSGT2RT+\nrCpSx4/VBEnkjWNHiDxpg8v+R70rfk/Fla4OndTRQ8Bnc+MUCH7lP59zuDMKz10/\nNIeWiu5T6CUVAgMBAAGjgbIwga8wDwYDVR0TAQH/BAUwAwEB/zAOBgNVHQ8BAf8E\nBAMCAQYwbQYIKwYBBQUHAQwEYTBfoV2gWzBZMFcwVRYJaW1hZ2UvZ2lmMCEwHzAH\nBgUrDgMCGgQUj+XTGoasjY5rw8+AatRIGCx7GS4wJRYjaHR0cDovL2xvZ28udmVy\naXNpZ24uY29tL3ZzbG9nby5naWYwHQYDVR0OBBYEFH/TZafC3ey78DAJ80M5+gKv\nMzEzMA0GCSqGSIb3DQEBBQUAA4IBAQCTJEowX2LP2BqYLz3q3JktvXf2pXkiOOzE\np6B4Eq1iDkVwZMXnl2YtmAl+X6/WzChl8gGqCBpH3vn5fJJaCGkgDdk+bW48DW7Y\n5gaRQBi5+MHt39tBquCWIMnNZBU4gcmU7qKEKQsTb47bDN0lAtukixlE0kF6BWlK\nWE9gyn6CagsCqiUXObXbf+eEZSqVir2G3l6BFoMtEMze/aiCKm0oHw0LxOXnGiYZ\n4fQRbxC1lfznQgUy286dUV4otp6F01vvpX1FQHKOtw5rDgb7MzVIcbidJ4vEZV8N\nhnacRHr2lVz2XTIIM6RUthg/aFzyQkqFOFSDX9HoLPKsEdao7WNq\n-----END
    CERTIFICATE-----\n"
globalconfigpollinterval: 3s
proxyconfigpollinterval: 1s
`

const proxiesYamlTemplate = `
fallback-104.236.192.114:
  addr: 104.236.192.114:443
  cert: "-----BEGIN CERTIFICATE-----\nMIIDoDCCAoigAwIBAgIEJSGW+TANBgkqhkiG9w0BAQsFADB4MQswCQYDVQQGEwJVUzETMBEGA1UE\nCBMKV2FzaGluZ3RvbjEQMA4GA1UEBxMHQWJ1c2VyczEhMB8GA1UEChMYUnVtcGVsc3RpbHRza2lu\nIEJsZW5kaW5nMR8wHQYDVQQDExZOYWlyIENvaW5hZ2VzIFJvb3RsZXNzMB4XDTE1MTIxMDE5MjI0\nNloXDTE2MTIwOTE5MjI0NloweDELMAkGA1UEBhMCVVMxEzARBgNVBAgTCldhc2hpbmd0b24xEDAO\nBgNVBAcTB0FidXNlcnMxITAfBgNVBAoTGFJ1bXBlbHN0aWx0c2tpbiBCbGVuZGluZzEfMB0GA1UE\nAxMWTmFpciBDb2luYWdlcyBSb290bGVzczCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB\nAKjJSnSh8ErJZhxppJLoee80dvMB0RB9xjjXkvhCB/k/PHSsGHzHQp2ywuD50RoMlFq5tBL10Nnx\nBQ8a56lqXdwAfJD74dN2ppA/wyzQ1KFRRp9kZb4l2jry6GArexHS1fnLEh6XgkEf4DLp4FdsBgWk\nB7EMJRh8HRYZLNZXZz+EUx+a9cIRFAoHDYg/CfzhqGk4qC2Wkoty/7LP72dIo4nC5ynzLNX/3HIQ\nTh+6Qt9KxJDC6OyWvputZc2bYcxeEVzx2/3FNsJtuuPiw8kIG1Ji6t+jlFJaE/82LweYPONmbiAd\nHIlXhFamy46dHAWgqG/iNPeQCkKyNbpS1/UAoKUCAwEAAaMyMDAwDwYDVR0RBAgwBocEaOzAcjAd\nBgNVHQ4EFgQUt93T+OqvcAvacIs3c0Qmj7KGp3AwDQYJKoZIhvcNAQELBQADggEBAJq5sYbq9wOl\nEcc87B56GJlLr6ZktGR7vQEvNsMq2YJwv1U4ZuSCuKx3IcuB4i+bvMWcaZRomNhiDbI07GLxYI3L\nSUbjJ4O/MJmUTb/KnmloRYPFPie6nq3sdAePCYwFUPLrz4RhOmII/nxWUqIoMvEFOHN+zRgr2s7n\npAFeLQ5PnWbPovfKCMi+imHlMSSBAWXQnLhfKUmkKfW1libcyV+MOjyhalQNFxHwkgNugLKlh7FN\nnvQl5FfTwrn5y+m3K5CIzFvnz3j7KdKtK3vfbA/Makbi4wc2/Gn2aMcFibFPBJyfSQ1QQkdRVtE4\n5lED/8D2Ekj3qRCtsGnuszJ6Fdk=\n-----END
    CERTIFICATE-----\n"
  authtoken: OlTIG6BtHaSDC2Iu2FmM7Xv3SYoP1HjjqpRXFGU7Q7719uRwJpQfTKPBfbN020SZ
  trusted: true
  pluggabletransport: ""
  pluggabletransportsettings: {}
  kcpsettings: {}
  enhttpurl: ""
`
