flashlight [![Travis CI Status](https://travis-ci.org/getlantern/flashlight.svg?branch=master)](https://travis-ci.org/getlantern/flashlight)&nbsp;[![Coverage Status](https://coveralls.io/repos/getlantern/flashlight/badge.png)](https://coveralls.io/r/getlantern/flashlight)&nbsp;[![GoDoc](https://godoc.org/github.com/getlantern/flashlight?status.png)](http://godoc.org/github.com/getlantern/flashlight)
==========

Lightweight host-spoofing web proxy written in go.

flashlight runs in one of two modes:

client - meant to run locally to wherever the browser is running, forwards
requests to the server

server - handles requests from a flashlight client proxy and actually proxies
them to the final destination

Using CloudFlare (and other CDNS), flashlight has the ability to masquerade as
running on a different domain than it is.  The client simply specifies the
"masquerade" flag with a value like "thehackernews.com".  flashlight will then
use that masquerade host for the DNS lookup and will also specify it as the
ServerName for SNI (though this is not actually necessary on CloudFlare). The
Host header of the HTTP request will actually contain the correct host
(e.g. getiantem.org), which causes CloudFlare to route the request to the
correct host.

Flashlight uses [enproxy](https://github.com/getlantern/enproxy) to encapsulate
data from/to the client as http request/response pairs.  This allows it to
tunnel regular HTTP as well as HTTPS traffic over CloudFlare.  In fact, it can
tunnel any TCP traffic.

### Usage

```bash
Usage of flashlight:
  -addr (required): ip:port on which to listen for requests.  When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https
  -configdir="": directory in which to store configuration (defaults to current directory)
  -cpuprofile="": write cpu profile to given file
  -dumpheaders=false: dump the headers of outgoing requests and responses to stdout
  -help=false: Get usage help
  -instanceid="": instanceId under which to report stats to statshub.  If not specified, no stats are reported.
  -masquerade="": masquerade host: if specified, flashlight will actually make a request to this host's IP but with a host header corresponding to the 'server' parameter
  -role (required): either 'client' or 'server'
  -rootca="": pin to this CA cert if specified (PEM format)
  -server (required): FQDN of flashlight server
  -serverport=443: the port on which to connect to the server
```

-rootca needs to be the complete PEM data, with header and trailer and all
newlines, for example:

```
flashlight -addr localhost:10080 -server localhost -serverport 10081 -rootca "-----BEGIN CERTIFICATE-----
MIIC/jCCAeigAwIBAgIEI6PHvjALBgkqhkiG9w0BAQswJjEQMA4GA1UEChMHTGFu
dGVybjESMBAGA1UEAxMJbG9jYWxob3N0MB4XDTE0MDUwMzE5NTQzMFoXDTI0MDYw
MzE5NTQzMFowJjEQMA4GA1UEChMHTGFudGVybjESMBAGA1UEAxMJbG9jYWxob3N0
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAzYeEJ/wJMeu0LA9/DLuw
n0j9HmAu/CK34e1jXsUuGkuheLLYWC32jVMsQdYaWuv8wFf2soXYH3WoEfOUkpTJ
N53WA4mmRd2nZidUxvUIiLdcQlJf+xar7vJih5MgsMYmVR+r7C1fLYlONuFpM6XV
5VuixGZyOcrLcOBbW1NimZLDzFYqAMy6l6U3eKvjK8KasnPURlAnVKRLquf4WA41
diQXWAzJCVgPz/f3Z4nL/SCADOkc2nGOroh63xbIra1eQdKfn8fOU1qeq/Bl1gPq
OdnSTGO19quSyf8XB6bDyl3TNeBCV5/FLIp8fjFzVdPAdZFjmMWTv3ccCEpmjsZe
xwIDAQABozgwNjAOBgNVHQ8BAf8EBAMCAKQwEwYDVR0lBAwwCgYIKwYBBQUHAwEw
DwYDVR0TAQH/BAUwAwEB/zALBgkqhkiG9w0BAQsDggEBAFLDvZBjdhLZuyHL3q6G
ZC93zaGkpdS8ux3gw4lldtr/SYW8aJ9Ck4+aGv7kouFylAAmxUXODUqh8vG1mc7D
uGHn5DHzHjlY1pSaedhcDcWIk1WB7ENoncWI9ZoutP3A4A+GTjwK35G7gBCP6bD+
qI6VIezWU0oFlFOgTdIKHNEbFpEgIUm1WUhrQ1zzRGVNVNxo4YZyqxe3pVKNwSmx
QggkGR2oOUVjfoyZ3pbUca4YnxiDgWRnbehgdK6Acq0kT9SCYAP0qTXCwZTeRJog
Na7vvprDERbUvc9c0rSUGHUrKqbf5AAmStI6fHGTNvdOMHZfoekwrE0CbyWcX/UH
gcA=
-----END CERTIFICATE-----"
```

**IMPORTANT** - when running a test locally, run the server first, then pass the
contents of servercert.pem to the client flashlight with the -rootca flag.  This
way the client will trust the local server, which is using a self-signed cert.

Example Client:

```bash
./flashlight -addr localhost:10080 -server getiantem.org -masquerade cdnjs.com
```

Example Server:

```bash
./flashlight -addr :443
```

Example Curl Test:

```bash
curl -x localhost:10080 http://www.google.com/humans.txt
Google is built by a large team of engineers, designers, researchers, robots, and others in many different sites across the globe. It is updated continuously, and built with more tools and technologies than we can shake a stick at. If you'd like to help us out, see google.com/careers.
```

On the client, you should see something like this for every request:

```bash
Handling request for: http://www.google.com/humans.txt
```

### Building

Flashlight requires [Go 1.3](http://golang.org/dl/).

It is convenient to build flashlight for multiple platforms using something like
[goxc](https://github.com/laher/goxc).

With goxc, the binaries used for Lantern can be built like this:

```
goxc -build-ldflags="-w" -bc="linux,386 linux,amd64 windows,386 darwin" validate compile
```

`-build-ldflags="-w"` causes the linker to omit debug symbols, which makes the
resulting binaries considerably smaller.

The binaries end up at
`$GOPATH/bin/flashlight-xc/snapshot/<platform>/flashlight`.
