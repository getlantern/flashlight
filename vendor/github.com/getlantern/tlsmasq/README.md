# TLS Masquerade

A server which masquerades as a different TLS server. For example, the server
may masquerade as a microsoft.com server, depsite not actually being run by
Microsoft.

Clients properly configured with the masquerade protocol can connect and speak
to the true server, but passive observers will see connections which look like
connections to microsoft.com. Similarly, active probes will find that the
server behaves like a microsoft.com server.
