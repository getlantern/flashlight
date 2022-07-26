module github.com/getlantern/flashlight

go 1.18

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.7.1-0.20220215050330-93bd217f5741

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20211116192935-1abdc4b1acab

replace github.com/keighl/mandrill => github.com/getlantern/mandrill v0.0.0-20191024010305-7094d8b40358

//replace github.com/getlantern/yinbi-server => ../yinbi-server

//replace github.com/getlantern/auth-server => ../auth-server

//replace github.com/getlantern/lantern-server => ../lantern-server

// For https://github.com/crawshaw/sqlite/pull/112 and https://github.com/crawshaw/sqlite/pull/103.
replace crawshaw.io/sqlite => github.com/getlantern/sqlite v0.0.0-20220301112206-cb2f8bc7cb56

replace github.com/eycorsican/go-tun2socks => github.com/getlantern/go-tun2socks v1.16.12-0.20201218023150-b68f09e5ae93

// We use a fork of gomobile that allows reusing the cache directory for faster builds, based
// on this unmerged PR against gomobile - https://github.com/golang/mobile/pull/58.
replace golang.org/x/mobile => github.com/oxtoacart/mobile v0.0.0-20220116191336-0bdf708b6d0f

// replace github.com/getlantern/dhtup => ../dhtup

require (
	git.torproject.org/pluggable-transports/goptlib.git v1.2.0
	github.com/PuerkitoBio/goquery v1.7.0
	github.com/anacrolix/dht/v2 v2.18.0
	github.com/anacrolix/go-libutp v1.2.0
	github.com/anacrolix/missinggo v1.3.0
	github.com/andybalholm/brotli v1.0.4
	github.com/blang/semver v3.5.1+incompatible
	github.com/dustin/go-humanize v1.0.0
	github.com/eycorsican/go-tun2socks v1.16.12-0.20201107203946-301549c435ff
	github.com/fsnotify/fsnotify v1.5.1
	github.com/getlantern/appdir v0.0.0-20200615192800-a0ef1968f4da
	github.com/getlantern/borda v0.0.0-20220308134056-c4a5602f778e
	github.com/getlantern/bufconn v0.0.0-20210901195825-fd7c0267b493
	github.com/getlantern/cmux/v2 v2.0.0-20200905031936-c55b16ee8462
	github.com/getlantern/cmuxprivate v0.0.0-20211216020409-d29d0d38be54
	github.com/getlantern/detour v0.0.0-20200814023224-28e20f4ac2d1
	github.com/getlantern/dhtup v0.0.0-20220627142103-ed614929df34
	github.com/getlantern/dnsgrab v0.0.0-20211216020425-5d5e155a01a8
	github.com/getlantern/domains v0.0.0-20220311111720-94f59a903271
	github.com/getlantern/ema v0.0.0-20190620044903-5943d28f40e4
	github.com/getlantern/enhttp v0.0.0-20210901195634-6f89d45ee033
	github.com/getlantern/errors v1.0.1
	github.com/getlantern/event v0.0.0-20210901195647-a7e3145142e6
	github.com/getlantern/eventual v1.0.0
	github.com/getlantern/eventual/v2 v2.0.2
	github.com/getlantern/fronted v0.0.0-20220707102541-0478ae49d9d3
	github.com/getlantern/geolookup v0.0.0-20210901195705-eec711834596
	github.com/getlantern/go-ping v0.0.0-20210901195920-5415d0f18231
	github.com/getlantern/go-socks5 v0.0.0-20171114193258-79d4dd3e2db5
	github.com/getlantern/golog v0.0.0-20211223150227-d4d95a44d873
	github.com/getlantern/hellosplitter v0.1.1
	github.com/getlantern/hidden v0.0.0-20220104173330-f221c5a24770
	github.com/getlantern/http-proxy-lantern/v2 v2.7.4
	github.com/getlantern/httpseverywhere v0.0.0-20201210200013-19ae11fc4eca
	github.com/getlantern/i18n v0.0.0-20181205222232-2afc4f49bb1c
	github.com/getlantern/idletiming v0.0.0-20201229174729-33d04d220c4e
	github.com/getlantern/iptool v0.0.0-20210901195942-5e13a4786de9
	github.com/getlantern/jibber_jabber v0.0.0-20210901195950-68955124cc42
	github.com/getlantern/kcpwrapper v0.0.0-20220503142841-b0e764933966
	github.com/getlantern/keyman v0.0.0-20210622061955-aa0d47d4932c
	github.com/getlantern/lampshade v0.0.0-20201109225444-b06082e15f3a
	github.com/getlantern/lantern-shadowsocks v1.3.6-0.20210601195915-e04471aa4920
	github.com/getlantern/measured v0.0.0-20210507000559-ec5307b2b8be
	github.com/getlantern/mitm v0.0.0-20210622063317-e6510574903b
	github.com/getlantern/mockconn v0.0.0-20200818071412-cb30d065a848
	github.com/getlantern/mtime v0.0.0-20200417132445-23682092d1f7
	github.com/getlantern/multipath v0.0.0-20220721104633-92b6c4b27b91
	github.com/getlantern/netx v0.0.0-20211206143627-7ccfeb739cbd
	github.com/getlantern/ops v0.0.0-20220622024155-46238aeb954f
	github.com/getlantern/osversion v0.0.0-20190510010111-432ecec19031
	github.com/getlantern/proxy/v2 v2.0.1-0.20220303164029-b34b76e0e581
	github.com/getlantern/proxybench v0.0.0-20220404140110-f49055cb86de
	github.com/getlantern/psmux v1.5.15-0.20200903210100-947ca5d91683
	github.com/getlantern/quicwrapper v0.0.0-20211104133553-140f96139f9f
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
	github.com/getlantern/tlsutil v0.5.2
	github.com/getlantern/uuid v1.2.0
	github.com/getlantern/waitforserver v1.0.1
	github.com/getlantern/yaml v0.0.0-20190801163808-0c9bb1ebf426
	github.com/getsentry/sentry-go v0.13.0
	github.com/google/gopacket v1.1.17
	github.com/hashicorp/golang-lru v0.5.4
	github.com/jaffee/commandeer v0.5.0
	github.com/keighl/mandrill v0.0.0-20170605120353-1775dd4b3b41
	github.com/mailgun/oxy v0.0.0-20180330141130-3a0f6c4b456b
	github.com/mitchellh/go-ps v1.0.0
	github.com/mitchellh/go-server-timing v1.0.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/pborman/uuid v1.2.1
	github.com/refraction-networking/utls v1.0.0
	github.com/shadowsocks/go-shadowsocks2 v0.1.4-0.20201002022019-75d43273f5a5
	github.com/stretchr/testify v1.8.0
	github.com/xtaci/smux v1.5.15-0.20200704123958-f7188026ba01
	gitlab.com/yawning/obfs4.git v0.0.0-20220204003609-77af0cba934d
	go.opentelemetry.io/otel v1.7.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.7.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.7.0
	go.opentelemetry.io/otel/sdk v1.7.0
	go.opentelemetry.io/otel/trace v1.7.0
	go.uber.org/atomic v1.9.0
	golang.org/x/mobile v0.0.0-20210831151748-9cba7bc03c0f
	golang.org/x/net v0.0.0-20220630215102-69896b714898
	golang.org/x/sys v0.0.0-20220704084225-05e143d24a9e
	google.golang.org/protobuf v1.28.0
	howett.net/plist v0.0.0-20200419221736-3b63eb3a43b5
)

require golang.org/x/exp v0.0.0-20220328175248-053ad81199eb // indirect

require (
	crawshaw.io/sqlite v0.3.3-0.20210127221821-98b1f83c5508 // indirect
	filippo.io/edwards25519 v1.0.0-rc.1.0.20210721174708-390f27c3be20 // indirect
	github.com/HdrHistogram/hdrhistogram-go v1.1.0 // indirect
	github.com/RoaringBitmap/roaring v0.9.4 // indirect
	github.com/Yawning/chacha20 v0.0.0-20170904085104-e3b1f968fc63 // indirect
	github.com/anacrolix/chansync v0.3.0 // indirect
	github.com/anacrolix/envpprof v1.1.1 // indirect
	github.com/anacrolix/log v0.13.2-0.20220426014722-7b7d13a55d55 // indirect
	github.com/anacrolix/missinggo/perf v1.0.0 // indirect
	github.com/anacrolix/missinggo/v2 v2.5.2 // indirect
	github.com/anacrolix/mmsg v1.0.0 // indirect
	github.com/anacrolix/multiless v0.2.0 // indirect
	github.com/anacrolix/squirrel v0.4.1-0.20220122230132-14b040773bac // indirect
	github.com/anacrolix/stm v0.3.0 // indirect
	github.com/anacrolix/sync v0.4.0 // indirect
	github.com/anacrolix/torrent v1.41.1-0.20220315024234-5a61d8f6ac93 // indirect
	github.com/anacrolix/upnp v0.1.3-0.20220123035249-922794e51c96 // indirect
	github.com/anacrolix/utp v0.1.0 // indirect
	github.com/andybalholm/cascadia v1.1.0 // indirect
	github.com/armon/go-radix v1.0.0 // indirect
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/benbjohnson/immutable v0.3.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.2.0 // indirect
	github.com/bradfitz/iter v0.0.0-20191230175014-e8f45d346db8 // indirect
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/cheekybits/genny v1.0.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dchest/siphash v1.2.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dsnet/compress v0.0.2-0.20210315054119-f66993602bf5 // indirect
	github.com/dvyukov/go-fuzz v0.0.0-20210429054444-fca39067bc72 // indirect
	github.com/edsrzf/mmap-go v1.0.0 // indirect
	github.com/felixge/httpsnoop v1.0.2 // indirect
	github.com/getlantern/bbrconn v0.0.0-20210901194755-12169918fdf9 // indirect
	github.com/getlantern/byteexec v0.0.0-20200509011419-2f5ed5531ada // indirect
	github.com/getlantern/bytemap v0.0.0-20210122162547-b07440a617f0 // indirect
	github.com/getlantern/cmux v0.0.0-20200905031936-c55b16ee8462 // indirect
	github.com/getlantern/context v0.0.0-20220418194847-3d5e7a086201 // indirect
	github.com/getlantern/dns v0.0.0-20210120185712-8d005533efa0 // indirect
	github.com/getlantern/elevate v0.0.0-20210901195629-ce58359e4d0e // indirect
	github.com/getlantern/enproxy v0.0.0-20180913191734-002212d417a4 // indirect
	github.com/getlantern/fdcount v0.0.0-20210503151800-5decd65b3731 // indirect
	github.com/getlantern/filepersist v0.0.0-20210901195658-ed29a1cb0b7c // indirect
	github.com/getlantern/framed v0.0.0-20190601192238-ceb6431eeede // indirect
	github.com/getlantern/geo v0.0.0-20210511192032-f563c52422c5 // indirect
	github.com/getlantern/go-cache v0.0.0-20141028142048-88b53914f467 // indirect
	github.com/getlantern/goexpr v0.0.0-20211215215226-4cdd4fd2847b // indirect
	github.com/getlantern/gonat v0.0.0-20201001145726-634575ba87fb // indirect
	github.com/getlantern/grtrack v0.0.0-20210901195719-bdf9e1d12dac // indirect
	github.com/getlantern/hex v0.0.0-20220104173244-ad7e4b9194dc // indirect
	github.com/getlantern/http-proxy v0.0.3-0.20211022035117-86faba795750 // indirect
	github.com/getlantern/kcp-go/v5 v5.0.0-20220503142114-f0c1cd6e1b54 // indirect
	github.com/getlantern/keepcurrent v0.0.0-20220716105246-a0f01ed60e13 // indirect
	github.com/getlantern/lantern-cloud v0.0.0-20220712165514-2ee1d944469a
	github.com/getlantern/msgpack v3.1.4+incompatible // indirect
	github.com/getlantern/packetforward v0.0.0-20201001150407-c68a447b0360 // indirect
	github.com/getlantern/pcapper v0.0.0-20210901200029-bf37dc0a4259 // indirect
	github.com/getlantern/preconn v1.0.0 // indirect
	github.com/getlantern/probe v0.0.0-20211216020459-69afa01c1c5c // indirect
	github.com/getlantern/probednet v0.0.0-20211216020507-22fd9c1d3bf6 // indirect
	github.com/getlantern/reconn v0.0.0-20161128113912-7053d017511c // indirect
	github.com/getlantern/ring v0.0.0-20210901200052-aea475211e37 // indirect
	github.com/getlantern/sqlparser v0.0.0-20171012210704-a879d8035f3c // indirect
	github.com/getlantern/testredis v0.0.0-20210901200107-a4ed71579e17 // indirect
	github.com/getlantern/wal v0.0.0-20220217194315-e4eac848dbd1 // indirect
	github.com/getlantern/withtimeout v0.0.0-20160829163843-511f017cd913 // indirect
	github.com/getlantern/zenodb v0.0.0-20220217194355-df46cc148a8c // indirect
	github.com/go-errors/errors v1.4.0 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-redis/redis/v8 v8.11.3 // indirect
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
	github.com/google/btree v1.0.1 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.10.1 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/juju/ratelimit v1.0.1 // indirect
	github.com/klauspost/compress v1.13.6 // indirect
	github.com/klauspost/cpuid v1.3.1 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/klauspost/reedsolomon v1.9.9 // indirect
	github.com/libp2p/go-buffer-pool v0.0.2 // indirect
	github.com/lucas-clemente/quic-go v0.19.3 // indirect
	github.com/marten-seemann/qtls-go1-16 v0.1.4 // indirect
	github.com/marten-seemann/qtls-go1-17 v0.1.0 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.0-beta.1 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/mdlayher/netlink v1.1.0 // indirect
	github.com/mholt/archiver/v3 v3.5.1 // indirect
	github.com/mikioh/tcp v0.0.0-20180707144002-02a37043a4f7 // indirect
	github.com/mikioh/tcpinfo v0.0.0-20180831101334-131b59fef27f // indirect
	github.com/mikioh/tcpopt v0.0.0-20180707144150-7178f18b4ea8 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mmcloughlin/avo v0.0.0-20200803215136-443f81d77104 // indirect
	github.com/montanaflynn/stats v0.6.3 // indirect
	github.com/mschoch/smat v0.2.0 // indirect
	github.com/nwaples/rardecode v1.1.0 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7 // indirect
	github.com/oschwald/geoip2-golang v1.5.0 // indirect
	github.com/oschwald/maxminddb-golang v1.8.0 // indirect
	github.com/oxtoacart/bpool v0.0.0-20190530202638-03653db5a59c // indirect
	github.com/pierrec/lz4/v4 v4.1.8 // indirect
	github.com/pion/datachannel v1.5.2 // indirect
	github.com/pion/dtls/v2 v2.1.2 // indirect
	github.com/pion/ice/v2 v2.1.20 // indirect
	github.com/pion/interceptor v0.1.7 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns v0.0.5 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.9 // indirect
	github.com/pion/rtp v1.7.4 // indirect
	github.com/pion/sctp v1.8.2 // indirect
	github.com/pion/sdp/v3 v3.0.4 // indirect
	github.com/pion/srtp/v2 v2.0.5 // indirect
	github.com/pion/stun v0.3.5 // indirect
	github.com/pion/transport v0.13.0 // indirect
	github.com/pion/turn/v2 v2.0.6 // indirect
	github.com/pion/udp v0.1.1 // indirect
	github.com/pion/webrtc/v3 v3.1.24-0.20220208053747-94262c1b2b38 // indirect
	github.com/pivotal-cf-experimental/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.12.2 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rs/dnscache v0.0.0-20210201191234-295bba877686 // indirect
	github.com/samber/lo v1.25.0
	github.com/siddontang/go v0.0.0-20180604090527-bdc77568d726 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/songgao/water v0.0.0-20200317203138-2b4b6d7c09d8 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/sparrc/go-ping v0.0.0-20190613174326-4e5b6552494c // indirect
	github.com/templexxx/cpu v0.0.8 // indirect
	github.com/templexxx/xorsimd v0.4.1 // indirect
	github.com/ti-mo/conntrack v0.3.0 // indirect
	github.com/ti-mo/netfilter v0.3.1 // indirect
	github.com/tjfoc/gmsm v1.3.2 // indirect
	github.com/tkuchiki/go-timezone v0.2.0 // indirect
	github.com/ulikunitz/xz v0.5.9 // indirect
	github.com/vulcand/oxy v0.0.0-20180330141130-3a0f6c4b456b // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/xwb1989/sqlparser v0.0.0-20180606152119-120387863bf2 // indirect
	gitlab.com/yawning/edwards25519-extra.git v0.0.0-20211229043746-2f91fcc9fbdb // indirect
	go.etcd.io/bbolt v1.3.6 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.7.0 // indirect
	go.opentelemetry.io/proto/otlp v0.16.0 // indirect
	go.uber.org/goleak v1.1.12 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.19.1 // indirect
	golang.org/x/crypto v0.0.0-20220622213112-05595931fe9d // indirect
	golang.org/x/mod v0.6.0-dev.0.20211013180041-c96bc1413d57 // indirect
	golang.org/x/sync v0.0.0-20220513210516-0976fa681c29 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20220609170525-579cf78fd858 // indirect
	golang.org/x/tools v0.1.8-0.20211029000441-d6a9af8af023 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220630174209-ad1d48641aa7 // indirect
	google.golang.org/grpc v1.47.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
