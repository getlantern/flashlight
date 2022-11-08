Multipath aggregates ordered and reliable connections over multiple paths together for throughput and resilience. It relies on existing dialers and listeners to create `net.Conn`s and wrap them as subflows on which it basically does two things:

1. On the sender side, transmits data over the subflow with the lowest roundtrip time, and if it takes long to get an acknowledgement, retransmits data over other subflows one by one.
1. On the receiver side, reorders the data received from all subflows and delivers ordered byte stream (`net.Conn`) to the upper layer.

See docs in [multipath.go](multipath.go) for details.

This code is used in https://github.com/benjojo/bondcat, and hat's off to [@benjojo](https://github.com/benjojo) for implementing [many fixes](https://github.com/benjojo/bondcat/tree/main/multipath).
