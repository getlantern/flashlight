module github.com/getlantern/flashlight

go 1.18

replace github.com/elazarl/goproxy => github.com/getlantern/goproxy v0.0.0-20220805074304-4a43a9ed4ec6

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.31.1-0.20230104154904-d810c964a217

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20221011213556-17014cb6fc4a

replace github.com/keighl/mandrill => github.com/getlantern/mandrill v0.0.0-20221004112352-e7c04248adcb

//replace github.com/getlantern/yinbi-server => ../yinbi-server

// replace github.com/getlantern/mandrill => /home/soltzen/dev/soltzen/mandrill

//replace github.com/getlantern/auth-server => ../auth-server

//replace github.com/getlantern/lantern-server => ../lantern-server

// For https://github.com/crawshaw/sqlite/pull/112 and https://github.com/crawshaw/sqlite/pull/103.
replace crawshaw.io/sqlite => github.com/getlantern/sqlite v0.0.0-20220301112206-cb2f8bc7cb56

replace github.com/eycorsican/go-tun2socks => github.com/getlantern/go-tun2socks v1.16.12-0.20201218023150-b68f09e5ae93

// We use a fork of gomobile that allows reusing the cache directory for faster builds, based
// on this unmerged PR against gomobile - https://github.com/golang/mobile/pull/58.
replace golang.org/x/mobile => github.com/oxtoacart/mobile v0.0.0-20220116191336-0bdf708b6d0f

replace github.com/Jigsaw-Code/outline-ss-server => github.com/getlantern/lantern-shadowsocks v1.3.6-0.20230114153732-0193919d4860

// replace github.com/getlantern/common => ../common

// replace github.com/getlantern/dhtup => ../dhtup

require (
	git.torproject.org/pluggable-transports/goptlib.git v1.2.0
	github.com/PuerkitoBio/goquery v1.7.0
	github.com/anacrolix/dht/v2 v2.19.2-0.20221121215055-066ad8494444
	github.com/anacrolix/go-libutp v1.2.0
	github.com/anacrolix/missinggo/v2 v2.7.0
	github.com/andybalholm/brotli v1.0.4
	github.com/blang/semver v3.5.1+incompatible
	github.com/dustin/go-humanize v1.0.0
	github.com/elazarl/goproxy v0.0.0-20221015165544-a0805db90819 // indirect
	github.com/eycorsican/go-tun2socks v1.16.12-0.20201107203946-301549c435ff
	github.com/fsnotify/fsnotify v1.5.4
	github.com/getlantern/appdir v0.0.0-20200615192800-a0ef1968f4da
	github.com/getlantern/borda v0.0.0-20220308134056-c4a5602f778e
	github.com/getlantern/bufconn v0.0.0-20210901195825-fd7c0267b493
	github.com/getlantern/cmux/v2 v2.0.0-20200905031936-c55b16ee8462
	github.com/getlantern/cmuxprivate v0.0.0-20211216020409-d29d0d38be54
	github.com/getlantern/detour v0.0.0-20200814023224-28e20f4ac2d1
	github.com/getlantern/dhtup v0.0.0-20230218063409-258bc7570a27
	github.com/getlantern/dnsgrab v0.0.0-20211216020425-5d5e155a01a8
	github.com/getlantern/domains v0.0.0-20220311111720-94f59a903271
	github.com/getlantern/ema v0.0.0-20190620044903-5943d28f40e4
	github.com/getlantern/enhttp v0.0.0-20210901195634-6f89d45ee033
	github.com/getlantern/errors v1.0.3
	github.com/getlantern/event v0.0.0-20210901195647-a7e3145142e6
	github.com/getlantern/eventual v1.0.0
	github.com/getlantern/eventual/v2 v2.0.2
	github.com/getlantern/fronted v0.0.0-20221102104652-893705395782
	github.com/getlantern/geolookup v0.0.0-20210901195705-eec711834596
	github.com/getlantern/go-socks5 v0.0.0-20171114193258-79d4dd3e2db5
	github.com/getlantern/golog v0.0.0-20221014032422-49749a7176cf
	github.com/getlantern/grtrack v0.0.0-20210901195719-bdf9e1d12dac // indirect
	github.com/getlantern/hellosplitter v0.1.1
	github.com/getlantern/hidden v0.0.0-20220104173330-f221c5a24770
	github.com/getlantern/http-proxy-lantern/v2 v2.8.1-0.20230215114357-157d6da42017
	github.com/getlantern/httpseverywhere v0.0.0-20201210200013-19ae11fc4eca
	github.com/getlantern/i18n v0.0.0-20181205222232-2afc4f49bb1c
	github.com/getlantern/idletiming v0.0.0-20201229174729-33d04d220c4e
	github.com/getlantern/iptool v0.0.0-20230112135223-c00e863b2696
	github.com/getlantern/jibber_jabber v0.0.0-20210901195950-68955124cc42
	github.com/getlantern/kcpwrapper v0.0.0-20220503142841-b0e764933966
	github.com/getlantern/keyman v0.0.0-20210622061955-aa0d47d4932c
	github.com/getlantern/lampshade v0.0.0-20201109225444-b06082e15f3a
	github.com/getlantern/measured v0.0.0-20210507000559-ec5307b2b8be
	github.com/getlantern/mitm v0.0.0-20210622063317-e6510574903b
	github.com/getlantern/mockconn v0.0.0-20200818071412-cb30d065a848
	github.com/getlantern/mtime v0.0.0-20200417132445-23682092d1f7
	github.com/getlantern/multipath v0.0.0-20220920195041-55195f38df73
	github.com/getlantern/netx v0.0.0-20211206143627-7ccfeb739cbd
	github.com/getlantern/ops v0.0.0-20220713155959-1315d978fff7
	github.com/getlantern/osversion v0.0.0-20230221120431-d6f9971f8ccf
	github.com/getlantern/proxy/v2 v2.0.1-0.20220303164029-b34b76e0e581
	github.com/getlantern/proxybench v0.0.0-20220404140110-f49055cb86de
	github.com/getlantern/psmux v1.5.15-0.20200903210100-947ca5d91683
	github.com/getlantern/quicproxy v0.0.0-20220808081037-32e9be8ec447
	github.com/getlantern/quicwrapper v0.0.0-20230124133216-09e62d6a4ff2
	github.com/getlantern/rot13 v0.0.0-20210901200056-01bce62cb8bb
	github.com/getlantern/rotator v0.0.0-20160829164113-013d4f8e36a2
	github.com/getlantern/safechannels v0.0.0-20201218194342-b4e5383e9627
	github.com/getlantern/shortcut v0.0.0-20211026183428-bf59a137fdec
	github.com/getlantern/timezone v0.0.0-20210901200113-3f9de9d360c9
	github.com/getlantern/tinywss v0.0.0-20211216020538-c10008a7d461
	github.com/getlantern/tlsdefaults v0.0.0-20171004213447-cf35cfd0b1b4
	github.com/getlantern/tlsdialer/v3 v3.0.3
	github.com/getlantern/tlsmasq v0.4.6
	github.com/getlantern/tlsresumption v0.0.0-20211216020551-6a3f901d86b9
	github.com/getlantern/tlsutil v0.5.3
	github.com/getlantern/uuid v1.2.0
	github.com/getlantern/waitforserver v1.0.1
	github.com/getlantern/yaml v0.0.0-20190801163808-0c9bb1ebf426
	github.com/getsentry/sentry-go v0.18.0
	github.com/google/gopacket v1.1.17
	github.com/hashicorp/golang-lru v0.5.4
	github.com/huin/goupnp v1.0.3 // indirect
	github.com/jaffee/commandeer v0.5.0
	github.com/keighl/mandrill v0.0.0-20170605120353-1775dd4b3b41
	github.com/lucas-clemente/quic-go v0.31.1 // indirect
	github.com/mailgun/oxy v0.0.0-20180330141130-3a0f6c4b456b
	github.com/mitchellh/go-ps v1.0.0
	github.com/mitchellh/go-server-timing v1.0.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/pborman/uuid v1.2.1
	github.com/refraction-networking/utls v1.0.0
	github.com/shadowsocks/go-shadowsocks2 v0.1.4-0.20201002022019-75d43273f5a5
	github.com/stretchr/testify v1.8.2
	github.com/vulcand/oxy v0.0.0-20180330141130-3a0f6c4b456b // indirect
	github.com/xtaci/smux v1.5.15-0.20200704123958-f7188026ba01
	gitlab.com/yawning/obfs4.git v0.0.0-20220204003609-77af0cba934d
	go.opentelemetry.io/otel v1.14.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.12.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.12.0
	go.opentelemetry.io/otel/sdk v1.14.0
	go.opentelemetry.io/otel/trace v1.14.0
	go.uber.org/atomic v1.9.0
	golang.org/x/mobile v0.0.0-20221110043201-43a038452099
	golang.org/x/net v0.7.0
	golang.org/x/sys v0.5.0
	google.golang.org/protobuf v1.28.1
	howett.net/plist v0.0.0-20200419221736-3b63eb3a43b5
)

require (
	github.com/Jigsaw-Code/outline-ss-server v1.4.0
	github.com/OperatorFoundation/Replicant-go/Replicant/v3 v3.0.14
	github.com/getlantern/broflake v0.0.0-20230420190113-3b1050a67a6d
	github.com/getlantern/common v1.2.1-0.20230423115230-85a56b8b6431
	github.com/getlantern/libp2p v0.0.0-20220913092210-f9e794d6b10d
)

require (
	github.com/OperatorFoundation/ghostwriter-go v1.0.6 // indirect
	github.com/OperatorFoundation/go-bloom v1.0.1 // indirect
	github.com/OperatorFoundation/go-shadowsocks2 v1.1.12 // indirect
	github.com/aead/ecdh v0.2.0 // indirect
	github.com/alecthomas/atomic v0.1.0-alpha2 // indirect
	github.com/getlantern/ratelimit v0.0.0-20220926192648-933ab81a6fc7 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/pprof v0.0.0-20220608213341-c488b8fa1db3 // indirect
	github.com/marten-seemann/qtls-go1-19 v0.1.1 // indirect
	github.com/miekg/dns v1.1.50 // indirect
	github.com/onsi/ginkgo/v2 v2.2.0 // indirect
	github.com/pion/transport/v2 v2.0.2 // indirect
	github.com/pion/udp/v2 v2.0.1 // indirect
	golang.org/x/exp v0.0.0-20220823124025-807a23277127 // indirect
	nhooyr.io/websocket v1.8.7 // indirect
)

require (
	crawshaw.io/sqlite v0.3.3-0.20220618202545-d1964889ea3c // indirect
	filippo.io/edwards25519 v1.0.0-rc.1.0.20210721174708-390f27c3be20 // indirect
	github.com/OperatorFoundation/Starbridge-go/Starbridge/v3 v3.0.12
	github.com/RoaringBitmap/roaring v1.2.1 // indirect
	github.com/Yawning/chacha20 v0.0.0-20170904085104-e3b1f968fc63 // indirect
	github.com/ajwerner/btree v0.0.0-20211221152037-f427b3e689c0 // indirect
	github.com/anacrolix/chansync v0.3.0 // indirect
	github.com/anacrolix/envpprof v1.2.1 // indirect
	github.com/anacrolix/generics v0.0.0-20220618083756-f99e35403a60 // indirect
	github.com/anacrolix/log v0.13.2-0.20221123232138-02e2764801c3 // indirect
	github.com/anacrolix/missinggo v1.3.0 // indirect
	github.com/anacrolix/missinggo/perf v1.0.0 // indirect
	github.com/anacrolix/mmsg v1.0.0 // indirect
	github.com/anacrolix/multiless v0.3.0 // indirect
	github.com/anacrolix/publicip v0.2.0 // indirect
	github.com/anacrolix/squirrel v0.4.1-0.20220122230132-14b040773bac // indirect
	github.com/anacrolix/stm v0.4.0 // indirect
	github.com/anacrolix/sync v0.4.0 // indirect
	github.com/anacrolix/torrent v1.48.1-0.20230103142631-c20f73d53e9f // indirect
	github.com/anacrolix/upnp v0.1.3-0.20220123035249-922794e51c96 // indirect
	github.com/anacrolix/utp v0.1.0 // indirect
	github.com/andybalholm/cascadia v1.1.0 // indirect
	github.com/armon/go-radix v1.0.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/benbjohnson/immutable v0.3.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.3.0 // indirect
	github.com/bradfitz/iter v0.0.0-20191230175014-e8f45d346db8 // indirect
	github.com/cenkalti/backoff/v4 v4.2.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dchest/siphash v1.2.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dsnet/compress v0.0.2-0.20210315054119-f66993602bf5 // indirect
	github.com/dvyukov/go-fuzz v0.0.0-20210429054444-fca39067bc72 // indirect
	github.com/edsrzf/mmap-go v1.1.0 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/getlantern/bbrconn v0.0.0-20210901194755-12169918fdf9 // indirect
	github.com/getlantern/byteexec v0.0.0-20220903142956-e6ed20032cfd // indirect
	github.com/getlantern/cmux v0.0.0-20200905031936-c55b16ee8462 // indirect
	github.com/getlantern/context v0.0.0-20220418194847-3d5e7a086201 // indirect
	github.com/getlantern/dns v0.0.0-20210120185712-8d005533efa0 // indirect
	github.com/getlantern/elevate v0.0.0-20220903142053-479ab992b264 // indirect
	github.com/getlantern/enproxy v0.0.0-20180913191734-002212d417a4 // indirect
	github.com/getlantern/fdcount v0.0.0-20210503151800-5decd65b3731 // indirect
	github.com/getlantern/filepersist v0.0.0-20210901195658-ed29a1cb0b7c // indirect
	github.com/getlantern/framed v0.0.0-20190601192238-ceb6431eeede // indirect
	github.com/getlantern/geo v0.0.0-20221101125300-c661769d5822 // indirect
	github.com/getlantern/go-cache v0.0.0-20141028142048-88b53914f467 // indirect
	github.com/getlantern/gonat v0.0.0-20201001145726-634575ba87fb // indirect
	github.com/getlantern/hex v0.0.0-20220104173244-ad7e4b9194dc // indirect
	github.com/getlantern/http-proxy v0.0.3-0.20230112154909-69209a6e2049 // indirect
	github.com/getlantern/kcp-go/v5 v5.0.0-20220503142114-f0c1cd6e1b54 // indirect
	github.com/getlantern/keepcurrent v0.0.0-20221014183517-fcee77376b89 // indirect
	github.com/getlantern/msgpack v3.1.4+incompatible // indirect
	github.com/getlantern/packetforward v0.0.0-20201001150407-c68a447b0360 // indirect
	github.com/getlantern/preconn v1.0.0 // indirect
	github.com/getlantern/probe v0.0.0-20211216020459-69afa01c1c5c // indirect
	github.com/getlantern/probednet v0.0.0-20211216020507-22fd9c1d3bf6 // indirect
	github.com/getlantern/reconn v0.0.0-20161128113912-7053d017511c // indirect
	github.com/getlantern/upnp v0.0.0-20220531140457-71a975af1fad // indirect
	github.com/getlantern/withtimeout v0.0.0-20160829163843-511f017cd913 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-redis/redis/v8 v8.11.5 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/golang/gddo v0.0.0-20190419222130-af0f2af80721 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/gonum/blas v0.0.0-20180125090452-e7c5890b24cf // indirect
	github.com/gonum/floats v0.0.0-20180125090339-7de1f4ea7ab5 // indirect
	github.com/gonum/internal v0.0.0-20180125090855-fda53f8d2571 // indirect
	github.com/gonum/lapack v0.0.0-20180125091020-f0b8b25edece // indirect
	github.com/gonum/matrix v0.0.0-20180124231301-a41cc49d4c29 // indirect
	github.com/gonum/stat v0.0.0-20180125090729-ec9c8a1062f4 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.10.2 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/klauspost/compress v1.15.12 // indirect
	github.com/klauspost/cpuid v1.3.1 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/klauspost/reedsolomon v1.9.9 // indirect
	github.com/libp2p/go-buffer-pool v0.0.2 // indirect
	github.com/lispad/go-generics-tools v1.1.0 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.3 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mdlayher/netlink v1.1.0 // indirect
	github.com/mholt/archiver/v3 v3.5.1 // indirect
	github.com/mikioh/tcp v0.0.0-20180707144002-02a37043a4f7 // indirect
	github.com/mikioh/tcpinfo v0.0.0-20180831101334-131b59fef27f // indirect
	github.com/mikioh/tcpopt v0.0.0-20180707144150-7178f18b4ea8 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mmcloughlin/avo v0.0.0-20200803215136-443f81d77104 // indirect
	github.com/mschoch/smat v0.2.0 // indirect
	github.com/nwaples/rardecode v1.1.2 // indirect
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7 // indirect
	github.com/oschwald/geoip2-golang v1.8.0 // indirect
	github.com/oschwald/maxminddb-golang v1.10.0 // indirect
	github.com/oxtoacart/bpool v0.0.0-20190530202638-03653db5a59c // indirect
	github.com/pierrec/lz4/v4 v4.1.12 // indirect
	github.com/pion/datachannel v1.5.5 // indirect
	github.com/pion/dtls/v2 v2.2.6 // indirect
	github.com/pion/ice/v2 v2.3.1 // indirect
	github.com/pion/interceptor v0.1.12 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns v0.0.7 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.10 // indirect
	github.com/pion/rtp v1.7.13 // indirect
	github.com/pion/sctp v1.8.6 // indirect
	github.com/pion/sdp/v3 v3.0.6 // indirect
	github.com/pion/srtp/v2 v2.0.12 // indirect
	github.com/pion/stun v0.4.0 // indirect
	github.com/pion/turn/v2 v2.1.0 // indirect
	github.com/pion/webrtc/v3 v3.1.58 // indirect
	github.com/pivotal-cf-experimental/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.13.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/rs/dnscache v0.0.0-20211102005908-e0241e321417 // indirect
	github.com/samber/lo v1.25.0
	github.com/siddontang/go v0.0.0-20180604090527-bdc77568d726 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/songgao/water v0.0.0-20200317203138-2b4b6d7c09d8 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/templexxx/cpu v0.0.8 // indirect
	github.com/templexxx/xorsimd v0.4.1 // indirect
	github.com/ti-mo/conntrack v0.3.0 // indirect
	github.com/ti-mo/netfilter v0.3.1 // indirect
	github.com/tidwall/btree v1.6.0 // indirect
	github.com/tjfoc/gmsm v1.3.2 // indirect
	github.com/tkuchiki/go-timezone v0.2.0 // indirect
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	gitlab.com/yawning/edwards25519-extra.git v0.0.0-20211229043746-2f91fcc9fbdb // indirect
	go.etcd.io/bbolt v1.3.6 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.13.0 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.21.0 // indirect
	golang.org/x/crypto v0.6.0 // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/sync v0.0.0-20220819030929-7fc1605a5dde // indirect
	golang.org/x/text v0.7.0 // indirect
	golang.org/x/time v0.0.0-20220922220347-f3bd1da661af // indirect
	golang.org/x/tools v0.1.12 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20221118155620-16455021b5e6 // indirect
	google.golang.org/grpc v1.52.3 // indirect
	gopkg.in/yaml.v3 v3.0.1
)
