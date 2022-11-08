UPnP utilities in Go

Maintainer: @soltzen

## Usage

See `cmd/main.go` for an example usage.

## Logging

The default logger is `logger.go:StdLogger`.

You can change this to your own logger by adding a struct that conforms to the `logger.go:Logger` interface and then calling `upnp.Logger = MyAwesomeCustomLogger{}`, or by suppressing all logs with `upnp.Logger = upnp.NoLogger{}`
