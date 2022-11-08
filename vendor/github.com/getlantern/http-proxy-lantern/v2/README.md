# HTTP/S Proxy with extensions for Lantern

[![Go Actions Status](https://github.com/getlantern/http-proxy-lantern/actions/workflows/go.yml/badge.svg)](https://github.com/getlantern/http-proxy-lantern/actions)

These are Lantern-specific middleware components for the HTTP Proxy in Go:

* A filter for access tokens

* A filter for devices, based on their IDs

* A filter for Pro users

* A connection preprocessor to intercept bad requests and send custom responses

* Custom responses for mimicking Apache in certain cases

### Building

Currently, http-proxy-lantern will only build on Linux hosts. You need to have
the package `libpcap-dev` installed.

### Usage

Build it with `go build` or with `make build`.

To get list of the command line options, please run `http-proxy-lantern -help`.

`config.ini.default` also has the list of options, make a copy (say, `config.ini`) and tweak it as you wish, then run the proxy with

```
http-proxy-lantern -config config.ini
```

To regenerate `config.ini.default` just run `http-proxy-lantern -dumpflags`.

### Testing with Lantern extensions and configuration

### Run tests

```
go test
```

Use this for verbose output:

```
TRACE=1 go test
```

### Manual testing

*Keep in mind that cURL doesn't support tunneling through an HTTPS proxy, so if you use the -https option you have to use other tools for testing.*

You can run a local server with a set configuration (just a default ReflectToSite proxy as of this writing) with

```
make local-proxy
```

Note that  `make local-proxy` is really just an alias for `make local-rts` -- i.e. a ReflectToSite local proxy.

You can then copy the rts-proxies.yaml file to your Lantern config directory, as in:

```
cp ./rts/rts-proxies.yaml ~/Library/Application\ Support/Lantern/proxies.yaml
```

Run a Lantern client accordingly from `lantern-desktop`, as in:

```
./lantern -readableconfig -stickyconfig
```

If you're developing a new transport, you can also add new versions of those files for that transport as you're testing.

You have two options to test it: the Lantern client or [checkfallbacks](https://github.com/getlantern/lantern/tree/valencia/src/github.com/getlantern/checkfallbacks).

Keep in mind that they will need to send some headers in order to avoid receiving 404 messages (the chained server response if you aren't providing them).

Currently, the only header you need to add is `X-Lantern-Device-Id`.

If you are using checkfallbacks, make sure that both the certificate and the token are correct.  A 404 will be the reply otherwise.  Running the server with `-debug` may help you troubleshooting those scenarios.

### Handle requests config server specially

[To prevent spoofers from fetching Lantern config with fake client IP](https://github.com/getlantern/config-server/issues/4), we need to attach auth tokens to such requests.  Both below options should be supplied. Once `http-proxy-lantern` receives GET request to one of the `cfgsvrdomains`, it sets `X-Lantern-Config-Auth-Token` header with supplied `cfgsvrauthtoken`, and `X-Lantern-Config-Client-IP` header with the IP address it sees.

```
  -cfgsvrauthtoken string
        Token attached to config-server requests, not attaching if empty
  -cfgsvrdomains string
        Config-server domains on which to attach auth token, separated by comma
```

### When something bad happens

With option `-pprofaddr=localhost:6060`, you can always access lots of debug information from http://localhost:6060/debug/pprof. Ref https://golang.org/pkg/net/http/pprof/.

***Be sure to only listen on localhost or private addresses for security reason.***

## Building for distribution and deploying

You can use the following command to do all this automatically. Note that `make dist` requires $VERSION. It will tag the repo with that version and will also generate a new changelog:

```
VERSION=v0.1.3 make dist
```

To build for distribution but not tagging or generating new changelog:

```
make distnochange
```

When building on non-linux machines, prepending `docker-` to the targets, i.e., `make docker-dist` and `make docker-distnochange` respectively.

Once you've build a binary for distribution, you can deploy it to all live proxies with:

```
make deploy
```

## Temporarily Deploying a Preview Binary to a Single Server
Sometimes it's useful to deploy a preview binary to a single server. This can
be done using either `deployTo.bash` or `onlyDeployTo.bash`. They do the same
thing but `deployTo.bash` first runs `make dist` whereas `onlyDeployTo.bash`
copies the existing binary at dist/http-proxy.

## Deploying a Custom Binary
Sometimes it's useful to deploy a custom binary to one or more tracks. This can
be done by running `make deploy-custom` and setting the environment variable
`BINARY_NAME` to the desired binary name, e.g.
`http-proxy-custom-hwh33-tlsmasq999`.

To deploy a track running the custom binary, add the `custom_proxy_binary` key
to the track's pillar data, mapped to the name specified above. At the time of
writing, track pillar data is specified in the `track_pillars` structure in
lantern-infrastructure/etc/current_production_track_config.py

### ssh config
Most of our proxies have `servermasq` enabled on them.
This means that you cannot ssh directly into them. Instead you have to use a cloudmaster as a bastion jump host.
You can do this relatively straightforwardly by adding this to your `~/.ssh/config` file:
```
Host bastion
  HostName CM_IP
  ProxyCommand none
Host *
  User lantern
  ProxyJump bastion
```
where you'd replace CM_IP with an actual cloudmaster ip (probably the one closest to you).


### Deploy Preview
```
./onlyDeployTo.bash <ip address>
```

### Revert to Production Binary
Once you're done checking out the preview, revert back to the production binary
with:

```
./revertToProductionBinary.bash <ip addres>
```

### Logs on Server
To view proxy logs on a given machine, run:

```
journalctl -e -u http-proxy
```

