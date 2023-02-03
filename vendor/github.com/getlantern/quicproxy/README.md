QUIC Forward and Reverse Proxies in Go

Maintainer: @soltzen

## Overview

A forward proxy (called `QuicForwardProxy` in this project) is a middleware that sits usually on the client and modifies **outgoing** connections before passing it to the internet.

A reverse proxy (called `QuicReverseProxy` in this project) is a middlware that sits usually on the server and modifies **incoming** connections before passing it to the server.

This project has both components and they can be used together thusly:

```
  Client  <--> QuicForwardProxy <---------> QuicReverseProxy <--> Internet

  ------------------------------          -----------------------------------
  ------------------------------          -----------------------------------
                 (1)                                       (2)

1. Client-side
  - This component implements an http.RoundTripper that takes an http.Request,
    does a complete roundtrip, and outputs an http.Response.
  - The http.Client uses QuicForwardProxy as its proxy and the request should magically
    be transformed into QUIC and can be received by any QuicReverseProxy.
  - QuicForwardProxy would send an HTTP CONNECT msg using QUIC as an underlying
    protocol to QuicReverseProxy

2. Server-side
  - This reverse proxy uses QUIC as a dialer and HTTP as the application-level
    protocol. It accepts only an HTTP CONNECT message and the HTTP traffic itself
    and sends it to the internet
```

## Usage

See `quicproxy_test.go` for a few examples.

See also `./cmd/` directory for executable examples.

## Logging

The default logger is `logger.go:StdLogger`.

You can change this to your own logger by adding a struct that conforms to the `logger.go:Logger` interface and then calling `quicproxy.Logger = MyAwesomeCustomLogger{}`, or by suppressing all logs with `quicproxy.Logger = quicproxy.NoLogger{}`

## FAQ

### Why is proxying HTTPS traffic taking a different code path than HTTP traffic?

This is how HTTP and HTTPS proxying differs:
- HTTPS: When proxying, the client fires a CONNECT request to the reverse
  proxy, which is then proxied to the remote server. The remote server
  responds with a 200 OK, and the reverse proxy responds with a 200 OK.
  The reverse proxy then forwards the request to the remote server.
- HTTP: When proxying, the client fires the SAME request (GET/POST/etc)
  to the proxy server (no CONNECT is done here). The proxy server then
  forwards the request to the remote server directly.

Because of this when QuicReverseProxy receives HTTP traffic, it is handled in `p.NonproxyHandler`, not in `p.ConnectDial`.

Ref: https://stackoverflow.com/a/34268925/3870025
