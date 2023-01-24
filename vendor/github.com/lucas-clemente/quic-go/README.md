This is a private fork.  To use, clone this repository as `$GOPATH/github.com/lucas-clemente/quic-go`
and perform the following additional setup locally:

```
git remote add upstream git@github.com:lucas-clemente/quic-go.git
git remote set-url --push upstream DISABLE
```

To take upstream changes (into master for example):

** Make sure to push a tag with the current master before executing a rebase to preserve any
references pointed at the current commit.

```
git fetch upstream
git tag snapshot_<version>
git push origin --tags
git rebase upstream/master
...
```

*Help!* I rebased and now something is complaining about a commit that doesn't exist!

You may still have the commit locally, just check it out, tag it and push the tag to the origin.
If you don't have it locally github likely still has it and you can use the api to create a branch
or tag using the commit hash.  See https://stackoverflow.com/questions/10098095/git-can-i-view-the-reflog-of-a-remote



If including this fork in a project using go modules, specify the following to use version X.Y.Z
```
replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go vX.Y.Z
```

This fork allows certain customizations and exposes some internal details that
are necessary for lantern/oquic development but may not be fully in spec
with QUIC or are not currently justifiable/understood needs upstream.

These include things like:
* making it possible to exclude SNI information
* exposing internal bandwidth estimates from congestion control algorithms
* making the quic packet size externally customizable

# A QUIC implementation in pure Go

<img src="docs/quic.png" width=303 height=124>

[![PkgGoDev](https://pkg.go.dev/badge/github.com/lucas-clemente/quic-go)](https://pkg.go.dev/github.com/lucas-clemente/quic-go)
[![Code Coverage](https://img.shields.io/codecov/c/github/lucas-clemente/quic-go/master.svg?style=flat-square)](https://codecov.io/gh/lucas-clemente/quic-go/)

quic-go is an implementation of the QUIC protocol ([RFC 9000](https://datatracker.ietf.org/doc/html/rfc9000), [RFC 9001](https://datatracker.ietf.org/doc/html/rfc9001), [RFC 9002](https://datatracker.ietf.org/doc/html/rfc9002)) in Go, including the Unreliable Datagram Extension ([RFC 9221](https://datatracker.ietf.org/doc/html/rfc9221)) and Datagram Packetization Layer Path MTU
   Discovery (DPLPMTUD, [RFC 8899](https://datatracker.ietf.org/doc/html/rfc8899)). It has support for HTTP/3 ([RFC 9114](https://datatracker.ietf.org/doc/html/rfc9114)), including QPACK ([RFC 9204](https://datatracker.ietf.org/doc/html/rfc9204)).

In addition to the RFCs listed above, it currently implements the [IETF QUIC draft-29](https://tools.ietf.org/html/draft-ietf-quic-transport-29). Support for draft-29 will eventually be dropped, as it is phased out of the ecosystem.

## Guides

*We currently support Go 1.18.x and Go 1.19.x.*

Running tests:

    go test ./...

### QUIC without HTTP/3

Take a look at [this echo example](example/echo/echo.go).

## Usage

### As a server

See the [example server](example/main.go). Starting a QUIC server is very similar to the standard lib http in go:

```go
http.Handle("/", http.FileServer(http.Dir(wwwDir)))
http3.ListenAndServeQUIC("localhost:4242", "/path/to/cert/chain.pem", "/path/to/privkey.pem", nil)
```

### As a client

See the [example client](example/client/main.go). Use a `http3.RoundTripper` as a `Transport` in a `http.Client`.

```go
http.Client{
  Transport: &http3.RoundTripper{},
}
```

## Projects using quic-go

| Project                                              | Description                                                                                            | Stars |
|------------------------------------------------------|--------------------------------------------------------------------------------------------------------|-------|
| [algernon](https://github.com/xyproto/algernon)      | Small self-contained pure-Go web server with Lua, Markdown, HTTP/2, QUIC, Redis and PostgreSQL support | ![GitHub Repo stars](https://img.shields.io/github/stars/xyproto/algernon?style=flat-square) |
| [caddy](https://github.com/caddyserver/caddy/)       | Fast, multi-platform web server with automatic HTTPS                                                   | ![GitHub Repo stars](https://img.shields.io/github/stars/caddyserver/caddy?style=flat-square) |
| [go-ipfs](https://github.com/ipfs/go-ipfs)           | IPFS implementation in go                                                                              | ![GitHub Repo stars](https://img.shields.io/github/stars/ipfs/go-ipfs?style=flat-square) |
| [syncthing](https://github.com/syncthing/syncthing/) | Open Source Continuous File Synchronization                                                            | ![GitHub Repo stars](https://img.shields.io/github/stars/syncthing/syncthing?style=flat-square) |
| [traefik](https://github.com/traefik/traefik)        | The Cloud Native Application Proxy                                                                     | ![GitHub Repo stars](https://img.shields.io/github/stars/traefik/traefik?style=flat-square) |
| [v2ray-core](https://github.com/v2fly/v2ray-core)    | A platform for building proxies to bypass network restrictions                                         | ![GitHub Repo stars](https://img.shields.io/github/stars/v2fly/v2ray-core?style=flat-square) |
| [cloudflared](https://github.com/cloudflare/cloudflared)    | A tunneling daemon that proxies traffic from the Cloudflare network to your origins             | ![GitHub Repo stars](https://img.shields.io/github/stars/cloudflare/cloudflared?style=flat-square) |
| [OONI Probe](https://github.com/ooni/probe-cli)            | The Open Observatory of Network Interference (OONI) aims to empower decentralized efforts in documenting Internet censorship around the world.   | ![GitHub Repo stars](https://img.shields.io/github/stars/ooni/probe-cli?style=flat-square) |
| [YoMo](https://github.com/yomorun/yomo)    | Streaming Serverless Framework for Geo-distributed System | ![GitHub Repo stars](https://img.shields.io/github/stars/yomorun/yomo?style=flat-square) |

## Contributing

We are always happy to welcome new contributors! We have a number of self-contained issues that are suitable for first-time contributors, they are tagged with [help wanted](https://github.com/lucas-clemente/quic-go/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22). If you have any questions, please feel free to reach out by opening an issue or leaving a comment.
