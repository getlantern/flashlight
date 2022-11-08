# tinywss

A module for establishing a rudimentary secure "websocket".
Performs websocket handshake, but does not actually
enforce the websocket protocol for data exchanged afterwards.
Exposes a dialer and listener returning objects conforming to
net.Conn and net.Listener.

It is not meant to be compatible with anything but itself.
