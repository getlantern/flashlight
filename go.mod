module github.com/getlantern/flashlight

require (
	git.torproject.org/pluggable-transports/goptlib.git v0.0.0-20180321061416-7d56ec4f381e
	git.torproject.org/pluggable-transports/obfs4.git v0.0.0-20180421031126-89c21805c212
	github.com/StackExchange/wmi v0.0.0-20180116203802-5d049714c4a6
	github.com/Yawning/chacha20 v0.0.0-20170904085104-e3b1f968fc63
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412
	github.com/anacrolix/go-libutp v1.0.1
	github.com/anacrolix/missinggo v1.1.0
	github.com/anacrolix/mmsg v1.0.0
	github.com/anacrolix/sync v0.0.0-20180808010631-44578de4e778
	github.com/aristanetworks/goarista v0.0.0-20190429220743-799535f6f364
	github.com/armon/go-radix v0.0.0-20170727155443-1fca145dffbc
	github.com/beorn7/perks v1.0.0
	github.com/blang/semver v0.0.0-20180305174007-c5e971dbed78
	github.com/cheekybits/genny v1.0.0
	github.com/cloudflare/sidh v0.0.0-20181111220428-fc8e6378752b
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd
	github.com/davecgh/go-spew v1.1.1
	github.com/dchest/siphash v1.1.0
	github.com/dustin/go-humanize v0.0.0-20180421182945-02af3965c54e
	github.com/fatih/set v0.2.1
	github.com/felixge/httpsnoop v1.0.0
	github.com/getlantern/appdir v0.0.0-20180320102544-7c0f9d241ea7
	github.com/getlantern/autoupdate v0.0.0-20180719190525-a22eab7ded99
	github.com/getlantern/bbrconn v0.0.0-20180619163322-86cf8c16f3d0
	github.com/getlantern/borda v0.0.0-20190507190350-9a0c110eea9d
	github.com/getlantern/bufconn v0.0.0-20190503112805-6402607914eb
	github.com/getlantern/byteexec v0.0.0-20170405023437-4cfb26ec74f4
	github.com/getlantern/bytemap v0.0.0-20180417025909-c7bf952233bc
	github.com/getlantern/cmux v0.0.0-20171023232237-ee58cae565e4
	github.com/getlantern/context v0.0.0-20190109183933-c447772a6520
	github.com/getlantern/detour v0.0.0-20190122161414-7107a599a170
	github.com/getlantern/dns v0.0.0-20170920204204-630ccc2c3041
	github.com/getlantern/dnsgrab v0.0.0-20171025121014-227f729dcb41
	github.com/getlantern/elevate v0.0.0-20180207094634-c2e2e4901072
	github.com/getlantern/ema v0.0.0-20180718025023-42474605965c
	github.com/getlantern/enhttp v0.0.0-20190401024120-a974fa851e3c
	github.com/getlantern/enproxy v0.0.0-20180913191734-002212d417a4
	github.com/getlantern/errors v0.0.0-20190325191628-abdb3e3e36f7
	github.com/getlantern/event v0.0.0-20170919023932-f16a5563f52e
	github.com/getlantern/eventual v0.0.0-20180125201821-84b02499361b
	github.com/getlantern/filepersist v0.0.0-20160317154340-c5f0cd24e799
	github.com/getlantern/framed v0.0.0-20190306221922-7f7919c8cf9b
	github.com/getlantern/fronted v0.0.0-20180905190541-5b4f46cc8f8d
	github.com/getlantern/geolookup v0.0.0-20180719190536-68d621f75f46
	github.com/getlantern/go-cache v0.0.0-20141028142048-88b53914f467
	github.com/getlantern/go-socks5 v0.0.0-20171114193258-79d4dd3e2db5
	github.com/getlantern/go-update v0.0.0-20190510022740-79c495ab728c
	github.com/getlantern/goexpr v0.0.0-20171209042432-610eae7c7314
	github.com/getlantern/golog v0.0.0-20170508214112-cca714f7feb5
	github.com/getlantern/gotun v0.0.0-20190422200803-35dee1b197b5
	github.com/getlantern/gowin v0.0.0-20160824205538-88fa116ddffc
	github.com/getlantern/hex v0.0.0-20190417191902-c6586a6fe0b7
	github.com/getlantern/hidden v0.0.0-20190325191715-f02dbb02be55
	github.com/getlantern/http-proxy v0.0.0-20181211022424-5b96142859a8
	github.com/getlantern/http-proxy-lantern v0.0.0-20190510153715-e005a9cd4425
	github.com/getlantern/httpseverywhere v0.0.0-20180326165025-9bdb93e40695
	github.com/getlantern/i18n v0.0.0-20181205222232-2afc4f49bb1c
	github.com/getlantern/idletiming v0.0.0-20190331182121-9540d1aeda2b
	github.com/getlantern/ipproxy v0.0.0-20190508162323-6329c3cbf2fa
	github.com/getlantern/iptool v0.0.0-20170421160045-8723ea29ea42
	github.com/getlantern/jibber_jabber v0.0.0-20160317154340-7346f98d2644
	github.com/getlantern/kcp-go v0.0.0-20171025115649-19559e0e938c
	github.com/getlantern/kcpwrapper v0.0.0-20171114192627-a35c895f6de7
	github.com/getlantern/keyman v0.0.0-20180207174507-f55e7280e93a
	github.com/getlantern/lampshade v0.0.0-20190507122828-84b870a67bd6
	github.com/getlantern/launcher v0.0.0-20160824210503-bc9fc3b11894
	github.com/getlantern/measured v0.0.0-20180919192309-c70b16bb4198
	github.com/getlantern/memhelper v0.0.0-20181113170838-777ea7552231
	github.com/getlantern/mitm v0.0.0-20180205214248-4ce456bae650
	github.com/getlantern/mockconn v0.0.0-20190403061815-a8ffa60494a6
	github.com/getlantern/msgpack v3.1.4+incompatible
	github.com/getlantern/mtime v0.0.0-20170117193331-ba114e4a82b0
	github.com/getlantern/netx v0.0.0-20190110220209-9912de6f94fd
	github.com/getlantern/notifier v0.0.0-20190514215959-14a5ab5a47d6
	github.com/getlantern/ops v0.0.0-20190325191751-d70cb0d6f85f
	github.com/getlantern/osversion v0.0.0-20180309120706-8f3fb296110c
	github.com/getlantern/packetforward v0.0.0-20190504050844-5be78b74b008
	github.com/getlantern/pcapper v0.0.0-20181212174440-a8b1a3ff0cde
	github.com/getlantern/preconn v0.0.0-20180328114929-0b5766010efe
	github.com/getlantern/profiling v0.0.0-20160317154340-2a15afbadcff
	github.com/getlantern/protected v0.0.0-20190111224713-cc3b5f4a0fb8
	github.com/getlantern/proxiedsites v0.0.0-20180805232824-5362487dd72c
	github.com/getlantern/proxy v0.0.0-20190225163220-31d1cc06ed3d
	github.com/getlantern/proxybench v0.0.0-20181017151515-2acfa62efd12
	github.com/getlantern/quicwrapper v0.0.0-20190110220323-f6dd70305d8e
	github.com/getlantern/reconn v0.0.0-20161128113912-7053d017511c
	github.com/getlantern/ring v0.0.0-20181206150603-dd46ce8faa01
	github.com/getlantern/rot13 v0.0.0-20160824200123-33f93fc1fe85
	github.com/getlantern/rotator v0.0.0-20160829164113-013d4f8e36a2
	github.com/getlantern/shortcut v0.0.0-20190117153616-bb4d4203cc25
	github.com/getlantern/sqlparser v0.0.0-20171012210704-a879d8035f3c
	github.com/getlantern/sysproxy v0.0.0-20171129134559-eb982eb14035
	github.com/getlantern/systray v0.0.0-20190131073753-26d5b920200d
	github.com/getlantern/tarfs v0.0.0-20171005185713-4987a6195239
	github.com/getlantern/tinywss v0.0.0-20190508231233-72f4ddc30925
	github.com/getlantern/tlsdefaults v0.0.0-20171004213447-cf35cfd0b1b4
	github.com/getlantern/tlsdialer v0.0.0-20180712141225-bae89e3a58a7
	github.com/getlantern/tlsredis v0.0.0-20180308045249-5d4ed6dd3836
	github.com/getlantern/uuid v1.2.1-0.20190515184524-7ab03de9f869
	github.com/getlantern/waitforserver v1.0.1
	github.com/getlantern/wal v0.0.0-20171207173857-028a06aadcb9
	github.com/getlantern/wfilter v0.0.0-20160829163852-69cc8585ee9c
	github.com/getlantern/winsvc v0.0.0-20160824205134-8bb3a5dbcc1d
	github.com/getlantern/withtimeout v0.0.0-20160829163843-511f017cd913
	github.com/getlantern/yaml v0.0.0-20160317154340-79303eb9c0d9
	github.com/getlantern/zenodb v0.0.0-20180618221016-44cc8585533a
	github.com/go-ole/go-ole v1.2.1
	github.com/go-stack/stack v1.8.0
	github.com/golang/gddo v0.0.0-20180703174436-daffe1f90ec5
	github.com/golang/groupcache v0.0.0-20180203143532-66deaeb636df
	github.com/golang/protobuf v1.2.0
	github.com/golang/snappy v0.0.0-20180518054509-2e65f85255db
	github.com/gonum/blas v0.0.0-20180125090452-e7c5890b24cf
	github.com/gonum/floats v0.0.0-20180125090339-7de1f4ea7ab5
	github.com/gonum/internal v0.0.0-20180125090855-fda53f8d2571
	github.com/gonum/lapack v0.0.0-20180125091020-f0b8b25edece
	github.com/gonum/matrix v0.0.0-20180124231301-a41cc49d4c29
	github.com/gonum/stat v0.0.0-20180125090729-ec9c8a1062f4
	github.com/google/btree v1.0.0
	github.com/google/gopacket v1.1.16
	github.com/google/uuid v1.1.1
	github.com/gorilla/websocket v0.0.0-20180306181548-eb925808374e
	github.com/hashicorp/golang-lru v0.0.0-20180201235237-0fb14efe8c47
	github.com/huandu/xstrings v1.2.0
	github.com/juju/ratelimit v1.0.1
	github.com/kardianos/osext v0.0.0-20170510131534-ae77be60afb1
	github.com/keighl/mandrill v0.0.0-20170605120353-1775dd4b3b41
	github.com/kr/binarydist v0.0.0-20160721043806-3035450ff8b9
	github.com/lucas-clemente/quic-go v0.7.1-0.20190207125157-7dc4be2ce994
	github.com/mailgun/oxy v0.0.0-20180330141130-3a0f6c4b456b
	github.com/marten-seemann/qtls-deprecated v0.0.0-20190207043627-591c71538704
	github.com/matttproud/golang_protobuf_extensions v1.0.1
	github.com/mdlayher/raw v0.0.0-20181016155347-fa5ef3332ca9
	github.com/miekg/dns v0.0.0-20180406150955-01d59357d468
	github.com/mikioh/tcp v0.0.0-20180313052821-a2964c74d73b
	github.com/mikioh/tcpinfo v0.0.0-20180210234901-fd8b9a80d71e
	github.com/mikioh/tcpopt v0.0.0-20180210233710-18f4a8218095
	github.com/mitchellh/go-homedir v0.0.0-20161203194507-b8bc1bf76747
	github.com/mitchellh/go-server-timing v0.0.0-20180226015900-d145200e1f90
	github.com/mitchellh/mapstructure v0.0.0-20180220230111-00c29f56e238
	github.com/mitchellh/panicwrap v0.0.0-20170106182340-fce601fe5557
	github.com/oschwald/geoip2-golang v1.2.1
	github.com/oschwald/maxminddb-golang v1.3.0
	github.com/oxtoacart/bpool v0.0.0-20190227141107-8c4636f812cc
	github.com/pborman/uuid v0.0.0-20180122190007-c65b2f87fee3
	github.com/petar/GoLLRB v0.0.0-20130427215148-53be0d36a84c
	github.com/pkg/errors v0.8.0
	github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_golang v0.9.2
	github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90
	github.com/prometheus/common v0.4.0
	github.com/prometheus/procfs v0.0.0-20190507164030-5867b95ac084
	github.com/refraction-networking/utls v0.0.0-20180627181930-e0edd7863bd2
	github.com/shirou/gopsutil v0.0.0-20180801053943-8048a2e9c577
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4
	github.com/sirupsen/logrus v1.2.0
	github.com/skratchdot/open-golang v0.0.0-20190402232053-79abb63cd66e
	github.com/stretchr/testify v1.3.0
	github.com/templexxx/cpufeat v0.0.0-20170927014610-3794dfbfb047
	github.com/templexxx/reedsolomon v0.0.0-20170927015403-7092926d7d05
	github.com/templexxx/xor v0.0.0-20170926022130-0af8e873c554
	github.com/tevino/abool v0.0.0-20170917061928-9b9efcf221b5
	github.com/tjfoc/gmsm v0.0.0-20180404022540-0effa9db1ba8
	github.com/vulcand/oxy v0.0.0-20180330141130-3a0f6c4b456b
	github.com/xtaci/smux v1.0.7
	github.com/xwb1989/sqlparser v0.0.0-20171128062118-da747e0c62c4
	golang.org/x/crypto v0.0.0-20190228161510-8dd112bcdc25
	golang.org/x/net v0.0.0-20181201002055-351d144fa1fc
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20190502175342-a43fa875dd82
	golang.org/x/text v0.3.0
	google.golang.org/appengine v1.1.0
	google.golang.org/genproto v0.0.0-20180817151627-c66870c02cf8
	google.golang.org/grpc v1.15.0
	gopkg.in/redis.v5 v5.2.9
)

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.7.1-0.20190514232624-e5e2953885df

replace github.com/google/netstack => github.com/getlantern/netstack v0.0.0-20190313202628-8999826b821d
