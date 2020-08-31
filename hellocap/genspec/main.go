// Command genspec generates a utls.ClientHelloSpec based on a sample hello captured from the
// system's default browser.
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"text/tabwriter"
	"time"

	"github.com/getlantern/flashlight/browsers"
	"github.com/getlantern/flashlight/hellocap"
	tls "github.com/refraction-networking/utls"
)

const tlsRecordHeaderLen = 5

var (
	timeout      = flag.Duration("timeout", 10*time.Second, "")
	tlsPrefix    = flag.Bool("tlsPrefix", false, "true: tls.SomeType; false: SomeType")
	launchServer = flag.Bool(
		"startServer", false,
		"true: launch a server to capture the hello; false: capture the hello from the default browser",
	)
)

// Disable logs from the standard library (in particular, 'http: TLS handshake error...')
func init() {
	log.SetOutput(ioutil.Discard)
}

var cipherSuitesToNames = map[uint16]string{
	// TLS 1.0 - 1.2 cipher suites.
	0x0005: "TLS_RSA_WITH_RC4_128_SHA",
	0x000a: "TLS_RSA_WITH_3DES_EDE_CBC_SHA",
	0x002f: "TLS_RSA_WITH_AES_128_CBC_SHA",
	0x0035: "TLS_RSA_WITH_AES_256_CBC_SHA",
	0x003c: "TLS_RSA_WITH_AES_128_CBC_SHA256",
	0x009c: "TLS_RSA_WITH_AES_128_GCM_SHA256",
	0x009d: "TLS_RSA_WITH_AES_256_GCM_SHA384",
	0xc007: "TLS_ECDHE_ECDSA_WITH_RC4_128_SHA",
	0xc009: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
	0xc00a: "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
	0xc011: "TLS_ECDHE_RSA_WITH_RC4_128_SHA",
	0xc012: "TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA",
	0xc013: "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
	0xc014: "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
	0xc023: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256",
	0xc027: "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
	0xc02f: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
	0xc02b: "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
	0xc030: "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
	0xc02c: "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
	0xcca8: "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
	0xcca9: "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305",

	// TLS 1.3 cipher suites.
	0x1301: "TLS_AES_128_GCM_SHA256",
	0x1302: "TLS_AES_256_GCM_SHA384",
	0x1303: "TLS_CHACHA20_POLY1305_SHA256",

	// indicator: "TLS_FALLBACK_SCSV",
	// that the client is doing version fallback. See RFC 7507.
	0x5600: "TLS_FALLBACK_SCSV",

	0xcc13: "OLD_TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
	0xcc14: "OLD_TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256",

	0xc024: "DISABLED_TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA384",
	0xc028: "DISABLED_TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA384",
	0x003d: "DISABLED_TLS_RSA_WITH_AES_256_CBC_SHA256",

	0xcc15: "FAKE_OLD_TLS_DHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
	0x009e: "FAKE_TLS_DHE_RSA_WITH_AES_128_GCM_SHA256",

	0x0033: "FAKE_TLS_DHE_RSA_WITH_AES_128_CBC_SHA",
	0x0039: "FAKE_TLS_DHE_RSA_WITH_AES_256_CBC_SHA",
	0x0004: "FAKE_TLS_RSA_WITH_RC4_128_MD5",
	0x00ff: "FAKE_TLS_EMPTY_RENEGOTIATION_INFO_SCSV",

	0x0a0a: "GREASE_PLACEHOLDER",
}

var versionsToNames = map[uint16]string{
	0x0300: "VersionSSL30",
	0x0301: "VersionTLS10",
	0x0302: "VersionTLS11",
	0x0303: "VersionTLS12",
	0x0304: "VersionTLS13",

	0x0a0a: "GREASE_PLACEHOLDER",
}

var curvesToNames = map[tls.CurveID]string{
	23: "CurveP256",
	24: "CurveP384",
	25: "CurveP521",
	29: "X25519",

	0x0a0a: "GREASE_PLACEHOLDER",
}

var sigAlgsToNames = map[tls.SignatureScheme]string{
	// RSASSA-PKCS1-v1_5 algorithms.
	0x0401: "PKCS1WithSHA256",
	0x0501: "PKCS1WithSHA384",
	0x0601: "PKCS1WithSHA512",

	// RSASSA-PSS algorithms with public key OID rsaEncryption.
	0x0804: "PSSWithSHA256",
	0x0805: "PSSWithSHA384",
	0x0806: "PSSWithSHA512",

	// ECDSA algorithms. Only constrained to a specific curve in TLS 1.3.
	0x0403: "ECDSAWithP256AndSHA256",
	0x0503: "ECDSAWithP384AndSHA384",
	0x0603: "ECDSAWithP521AndSHA512",

	// Legacy signature and hash algorithms for TLS 1.2.
	0x0201: "PKCS1WithSHA1",
	0x0203: "ECDSAWithSHA1",
}

var pskModesToNames = map[uint8]string{
	tls.PskModePlain: "PskModePlain",
	tls.PskModeDHE:   "PskModeDHE",
}

type extensionInfo struct {
	name    string
	weblink string
}

var additionalKnownExtensions = map[uint16]extensionInfo{
	24: {"Token Binding", "https://tools.ietf.org/html/rfc8472"},
	27: {"Certificate Compression", "https://tools.ietf.org/html/draft-ietf-tls-certificate-compression-10"},
	28: {"Record Size Limit", "https://tools.ietf.org/html/rfc8449"},
}

func defaultBrowserSpec(timeout time.Duration) (*tls.ClientHelloSpec, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	b, err := browsers.SystemDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain default browser: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Default browser identified as", b)

	hello, err := hellocap.GetDefaultBrowserHello(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to capture hello: %w", err)
	}

	spec, err := tls.FingerprintClientHello(hello[tlsRecordHeaderLen:])
	if err != nil {
		return nil, fmt.Errorf("failed to fingerprint captured hello: %w", err)
	}
	return spec, nil
}

func marshalAsCode(spec tls.ClientHelloSpec, w io.Writer, tlsPrefix bool) {
	w = tabwriter.NewWriter(w, 4, 4, 1, ' ', 0)
	tlsName := func(name string) string {
		if tlsPrefix {
			return fmt.Sprintf("tls.%s", name)
		}
		return name
	}

	if spec.GetSessionID != nil {
		panic("expected GetSessionID to be nil")
	}

	fmt.Fprintf(w, "%s{\n", tlsName("ClientHelloSpec"))
	fmt.Fprintln(w, "\tCipherSuites: []uint16{")
	for _, suite := range spec.CipherSuites {
		name, ok := cipherSuitesToNames[suite]
		if !ok {
			fmt.Fprintf(w, "\t\t%#x, // unrecognized cipher suite\n", suite)
		} else {
			fmt.Fprintf(w, "\t\t%s,\n", tlsName(name))
		}
	}
	fmt.Fprintln(w, "\t},")
	fmt.Fprintln(w, "\tCompressionMethods: []uint8{")
	for _, cm := range spec.CompressionMethods {
		fmt.Fprintf(w, "\t\t%#x,", cm)
		if cm == 0 {
			fmt.Fprintf(w, " // no compression")
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "\t},")
	fmt.Fprintln(w, "\tExtensions: []tls.TLSExtension{")
	for _, ext := range spec.Extensions {
		printEmptyStruct := func(name string) {
			fmt.Fprintf(w, "\t\t&%s{},\n", tlsName(name))
		}

		switch typedExt := ext.(type) {
		case *tls.SNIExtension:
			printEmptyStruct("SNIExtension")
		case *tls.NPNExtension:
			printEmptyStruct("NPNExtension")
		case *tls.StatusRequestExtension:
			printEmptyStruct("StatusRequestExtension")
		case *tls.SupportedCurvesExtension:
			fmt.Fprintf(w, "\t\t&%s{\n", tlsName("SupportedCurvesExtension"))
			fmt.Fprintf(w, "\t\t\tCurves: []%s{\n", tlsName("CurveID"))
			for _, cID := range typedExt.Curves {
				name, ok := curvesToNames[cID]
				if !ok {
					fmt.Fprintf(w, "\t\t\t\t%d, // unrecognized curve ID\n", cID)
				} else {
					fmt.Fprintf(w, "\t\t\t\t%s,\n", tlsName(name))
				}
			}
			fmt.Fprintln(w, "\t\t\t},")
			fmt.Fprintln(w, "\t\t},")
		case *tls.SupportedPointsExtension:
			fmt.Fprintf(w, "\t\t&%s{\n", tlsName("SupportedPointsExtension"))
			fmt.Fprintln(w, "\t\t\tSupportedPoints: []uint8{")
			for _, pt := range typedExt.SupportedPoints {
				fmt.Fprintf(w, "\t\t\t\t%#x,", pt)
				if pt == 0 {
					fmt.Fprint(w, " // pointFormatUncompressed")
				}
				fmt.Fprintln(w)
			}
			fmt.Fprintln(w, "\t\t\t},")
			fmt.Fprintln(w, "\t\t},")
		case *tls.SessionTicketExtension:
			printEmptyStruct("SessionTicketExtension")
		case *tls.SignatureAlgorithmsExtension:
			fmt.Fprintf(w, "\t\t&%s{\n", tlsName("SignatureAlgorithmsExtension"))
			fmt.Fprintf(w, "\t\t\tSupportedSignatureAlgorithms: []%s{\n", tlsName("SignatureScheme"))
			for _, alg := range typedExt.SupportedSignatureAlgorithms {
				name, ok := sigAlgsToNames[alg]
				if !ok {
					fmt.Fprintf(w, "\t\t\t\t%#x, // unrecognized signature scheme\n", alg)
				} else {
					fmt.Fprintf(w, "\t\t\t\t%s,\n", tlsName(name))
				}
			}
			fmt.Fprintln(w, "\t\t\t},")
			fmt.Fprintln(w, "\t\t},")
		case *tls.RenegotiationInfoExtension:
			fmt.Fprintf(w, "\t\t&%s{\n", tlsName("RenegotiationInfoExtension"))
			fmt.Fprintf(w, "\t\t\tRenegotiation: %s,\n", tlsName("RenegotiateOnceAsClient"))
			fmt.Fprintln(w, "\t\t},")
		case *tls.ALPNExtension:
			fmt.Fprintf(w, "\t\t&%s{\n", tlsName("ALPNExtension"))
			fmt.Fprintln(w, "\t\t\tAlpnProtocols: []string{")
			for _, p := range typedExt.AlpnProtocols {
				fmt.Fprintf(w, "\t\t\t\t\"%s\",\n", p)
			}
			fmt.Fprintln(w, "\t\t\t},")
			fmt.Fprintln(w, "\t\t},")
		case *tls.SCTExtension:
			printEmptyStruct("SCTExtension")
		case *tls.SupportedVersionsExtension:
			fmt.Fprintf(w, "\t\t&%s{\n", tlsName("SupportedVersionsExtension"))
			fmt.Fprintln(w, "\t\t\tVersions: []uint16{")
			for _, v := range typedExt.Versions {
				name, ok := versionsToNames[v]
				if !ok {
					fmt.Fprintf(w, "\t\t\t\t%#x, // unrecognized version\n", v)
				} else {
					fmt.Fprintf(w, "\t\t\t\t%s,\n", tlsName(name))
				}
			}
			fmt.Fprintln(w, "\t\t\t},")
			fmt.Fprintln(w, "\t\t},")
		case *tls.KeyShareExtension:
			fmt.Fprintf(w, "\t\t&%s{\n", tlsName("KeyShareExtension"))
			fmt.Fprintf(w, "\t\t\tKeyShares: []%s{\n", tlsName("KeyShare"))
			for _, ks := range typedExt.KeyShares {
				fmt.Fprintln(w, "\t\t\t\t{")
				curveName, ok := curvesToNames[ks.Group]
				if !ok {
					fmt.Fprintf(w, "\t\t\t\t\tGroup: %d, // unrecognized curve ID\n", ks.Group)
				} else {
					fmt.Fprintf(w, "\t\t\t\t\tGroup: %s,\n", tlsName(curveName))
				}
				// Don't include keyshare data unless this is GREASE.
				if ks.Group == tls.GREASE_PLACEHOLDER {
					fmt.Fprintln(w, "\t\t\t\t\tData: []byte{")
					printBytes(ks.Data, w, 6, 10)
					fmt.Fprintln(w)
					fmt.Fprintln(w, "\t\t\t\t\t}")
				}
				fmt.Fprintln(w, "\t\t\t\t},")
			}
			fmt.Fprintln(w, "\t\t\t},")
			fmt.Fprintln(w, "\t\t},")
		case *tls.PSKKeyExchangeModesExtension:
			fmt.Fprintf(w, "\t\t&%s{\n", tlsName("PSKKeyExchangeModesExtension"))
			fmt.Fprintln(w, "\t\t\tModes: []uint8{")
			for _, mode := range typedExt.Modes {
				name, ok := pskModesToNames[mode]
				if !ok {
					fmt.Fprintf(w, "\t\t\t\t%d, // unrecognized mode\n", mode)
				} else {
					fmt.Fprintf(w, "\t\t\t\t%s,\n", tlsName(name))
				}
			}
			fmt.Fprintln(w, "\t\t\t},")
			fmt.Fprintln(w, "\t\t},")
		case *tls.UtlsExtendedMasterSecretExtension:
			printEmptyStruct("UtlsExtendedMasterSecretExtension")
		case *tls.UtlsPaddingExtension:
			// Compare the addresses of the function values to see if we got BoringPaddingStyle.
			if fmt.Sprint(typedExt.GetPaddingLen) != fmt.Sprint(tls.BoringPaddingStyle) {
				panic("expected BoringPaddingStyle")
			}
			fmt.Fprintf(w, "\t\t&%s{\n", tlsName("UtlsPaddingExtension"))
			fmt.Fprintf(w, "\t\t\tGetPaddingLen: %s,\n", tlsName("BoringPaddingStyle"))
			fmt.Fprintln(w, "\t\t},")
		case *tls.GenericExtension:
			fmt.Fprintf(w, "\t\t&%s{\n", tlsName("GenericExtension"))
			if info, ok := additionalKnownExtensions[typedExt.Id]; ok {
				fmt.Fprintf(w, "\t\t\t// %s:\n", info.name)
				fmt.Fprintf(w, "\t\t\t// %s\n", info.weblink)
			} else {
				fmt.Fprintln(w, "\t\t\t// XXX: unknown extension")
			}
			fmt.Fprintf(w, "\t\t\tId: %d,\n", typedExt.Id)
			fmt.Fprintln(w, "\t\t\tData: []byte{")
			printBytes(typedExt.Data, w, 4, 10)
			fmt.Fprintln(w)
			fmt.Fprintln(w, "\t\t\t},")
			fmt.Fprintln(w, "\t\t},")
		case *tls.UtlsGREASEExtension:
			printEmptyStruct("UtlsGREASEExtension")
		default:
			panic(fmt.Sprintf("unknown extension type %T", ext))
		}
	}
	fmt.Fprintln(w, "\t},")
	if spec.TLSVersMin != 0 {
		name, ok := versionsToNames[spec.TLSVersMin]
		if !ok {
			fmt.Fprintf(w, "\tTLSVersMin: %#x, // unrecognized version\n", spec.TLSVersMin)
		} else {
			fmt.Fprintf(w, "\tTLSVersMin: %s,\n", tlsName(name))
		}
	}
	if spec.TLSVersMax != 0 {
		name, ok := versionsToNames[spec.TLSVersMax]
		if !ok {
			fmt.Fprintf(w, "\tTLSVersMax: %#x, // unrecognized version\n", spec.TLSVersMax)
		} else {
			fmt.Fprintf(w, "\tTLSVersMax: %s,\n", tlsName(name))
		}
	}
	fmt.Fprintln(w, "}")
}

// The bytes will start on the current line. When this function returns, w will be at the end of the
// last line of bytes.
func printBytes(b []byte, w io.Writer, indentationLevel, bytesPerLine int) {
	if len(b) == 0 {
		return
	}

	printTabs := func() {
		for i := 0; i < indentationLevel; i++ {
			fmt.Fprint(w, "\t")
		}
	}

	printTabs()
	for i := 0; i < len(b); i++ {
		if i > 0 && i%bytesPerLine == 0 {
			fmt.Fprint(w, ",\n")
			printTabs()
		} else if i > 0 {
			fmt.Fprint(w, ", ")
		}
		fmt.Fprintf(w, "%d", b[i])
	}
	fmt.Fprint(w, ",")
}

func fail(a ...interface{}) {
	fmt.Fprintln(os.Stderr, a...)
	os.Exit(1)
}

func main() {
	flag.Parse()

	if !*launchServer {
		spec, err := defaultBrowserSpec(*timeout)
		if err != nil {
			fail(err)
		}
		marshalAsCode(*spec, os.Stdout, *tlsPrefix)
		return
	}

	onHello := func(hello []byte, err error) {
		if err != nil {
			fmt.Fprintln(os.Stderr, "error capturing hello:", err)
			return
		}
		spec, err := tls.FingerprintClientHello(hello[tlsRecordHeaderLen:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to fingerprint captured hello:", err)
			fmt.Fprintln(os.Stderr, "here is the hello we failed to fingerprint, base64-encoded:")
			fmt.Fprintln(os.Stderr, base64.StdEncoding.EncodeToString(hello))
			return
		}
		marshalAsCode(*spec, os.Stdout, *tlsPrefix)
	}

	s, err := hellocap.NewServer(onHello)
	if err != nil {
		fail("failed to start server:", err)
	}
	defer s.Close()

	// Grab the port and encourage use of localhost so that SNIs are included.
	_, port, err := net.SplitHostPort(s.Addr().String())
	if err != nil {
		fail("failed to parse server address:", err)
	}

	fmt.Fprintf(os.Stderr, "listening on https://localhost:%s\n", port)
	if err := s.Start(); err != nil {
		fail("server error:", err)
	}
}
