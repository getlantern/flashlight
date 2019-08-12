module github.com/getlantern/flashlight

go 1.12

require (
	cloud.google.com/go v0.44.0 // indirect
	contrib.go.opencensus.io/exporter/stackdriver v0.6.0 // indirect
	git.apache.org/thrift.git v0.0.0-20180902110319-2566ecd5d999 // indirect
	git.torproject.org/pluggable-transports/goptlib.git v0.0.0-20180321061416-7d56ec4f381e
	git.torproject.org/pluggable-transports/obfs4.git v0.0.0-20180421031126-89c21805c212
	github.com/RoaringBitmap/roaring v0.4.17 // indirect
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412 // indirect
	github.com/anacrolix/envpprof v1.0.0 // indirect
	github.com/anacrolix/go-libutp v1.0.1
	github.com/anacrolix/missinggo v1.1.0 // indirect
	github.com/anacrolix/mmsg v1.0.0 // indirect
	github.com/anacrolix/tagflag v1.0.0 // indirect
	github.com/armon/go-radix v0.0.0-20170727155443-1fca145dffbc // indirect
	github.com/beorn7/perks v1.0.0 // indirect
	github.com/blang/semver v0.0.0-20180723201105-3c1074078d32 // indirect
	github.com/bradfitz/iter v0.0.0-20190303215204-33e6a9893b0c // indirect
	github.com/cloudflare/sidh v0.0.0-20190228162259-d2f0f90e08aa // indirect
	github.com/cloudfoundry/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/dchest/siphash v1.2.0 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/set v0.2.1 // indirect
	github.com/felixge/httpsnoop v1.0.0 // indirect
	github.com/getlantern/appdir v0.0.0-20180320102544-7c0f9d241ea7
	github.com/getlantern/autoupdate v0.0.0-20180719190525-a22eab7ded99
	github.com/getlantern/bbrconn v0.0.0-20180619163322-86cf8c16f3d0 // indirect
	github.com/getlantern/borda v0.0.0-20190809122504-668025f4c2b9
	github.com/getlantern/bufconn v0.0.0-20190625204133-a08544339f8d
	github.com/getlantern/cmux v0.0.0-20190711013109-98b1e3bae67b
	github.com/getlantern/detour v0.0.0-20190122161414-7107a599a170
	github.com/getlantern/dns v0.0.0-20170920204204-630ccc2c3041 // indirect
	github.com/getlantern/dnsgrab v0.0.0-20171025121014-227f729dcb41
	github.com/getlantern/ema v0.0.0-20190620044903-5943d28f40e4
	github.com/getlantern/enhttp v0.0.0-20190401024120-a974fa851e3c
	github.com/getlantern/enproxy v0.0.0-20180913191734-002212d417a4 // indirect
	github.com/getlantern/errors v0.0.0-20190325191628-abdb3e3e36f7
	github.com/getlantern/event v0.0.0-20170919023932-f16a5563f52e
	github.com/getlantern/eventual v0.0.0-20180125201821-84b02499361b
	github.com/getlantern/filepersist v0.0.0-20160317154340-c5f0cd24e799
	github.com/getlantern/fronted v0.0.0-20190606212108-e7744195eded
	github.com/getlantern/geolookup v0.0.0-20180719190536-68d621f75f46
	github.com/getlantern/go-cache v0.0.0-20141028142048-88b53914f467 // indirect
	github.com/getlantern/go-socks5 v0.0.0-20171114193258-79d4dd3e2db5
	github.com/getlantern/go-update v0.0.0-20190510022740-79c495ab728c // indirect
	github.com/getlantern/golog v0.0.0-20190809085441-26e09e6dd330
	github.com/getlantern/gotun v0.0.0-20190523194503-885514e382d2
	github.com/getlantern/gowin v0.0.0-20160824205538-88fa116ddffc // indirect
	github.com/getlantern/hidden v0.0.0-20190325191715-f02dbb02be55
	github.com/getlantern/http-proxy v0.0.0-20190321015922-4801e21d5eca // indirect
	github.com/getlantern/http-proxy-lantern v0.1.3
	github.com/getlantern/httpseverywhere v0.0.0-20180326165025-9bdb93e40695
	github.com/getlantern/i18n v0.0.0-20181205222232-2afc4f49bb1c
	github.com/getlantern/idletiming v0.0.0-20190529182719-d2fbc83372a5
	github.com/getlantern/ipproxy v0.0.0-20190508162323-6329c3cbf2fa
	github.com/getlantern/iptool v0.0.0-20170421160045-8723ea29ea42
	github.com/getlantern/jibber_jabber v0.0.0-20160317154340-7346f98d2644
	github.com/getlantern/kcp-go v0.0.0-20171025115649-19559e0e938c // indirect
	github.com/getlantern/kcpwrapper v0.0.0-20171114192627-a35c895f6de7
	github.com/getlantern/keyman v0.0.0-20180207174507-f55e7280e93a
	github.com/getlantern/lampshade v0.0.0-20190812172212-64223f525a57
	github.com/getlantern/launcher v0.0.0-20160824210503-bc9fc3b11894
	github.com/getlantern/measured v0.0.0-20180919192309-c70b16bb4198
	github.com/getlantern/memhelper v0.0.0-20181113170838-777ea7552231
	github.com/getlantern/mitm v0.0.0-20180205214248-4ce456bae650
	github.com/getlantern/mockconn v0.0.0-20190708122800-637bd46d8034
	github.com/getlantern/mtime v0.0.0-20170117193331-ba114e4a82b0
	github.com/getlantern/netx v0.0.0-20190110220209-9912de6f94fd
	github.com/getlantern/notifier v0.0.0-20190531021800-64e5c4112f43
	github.com/getlantern/ops v0.0.0-20190325191751-d70cb0d6f85f
	github.com/getlantern/osversion v0.0.0-20180309120706-8f3fb296110c
	github.com/getlantern/packetforward v0.0.0-20190617161814-583f227df593
	github.com/getlantern/pcapper v0.0.0-20181212174440-a8b1a3ff0cde // indirect
	github.com/getlantern/preconn v0.0.0-20180328114929-0b5766010efe // indirect
	github.com/getlantern/profiling v0.0.0-20160317154340-2a15afbadcff
	github.com/getlantern/protected v0.0.0-20190111224713-cc3b5f4a0fb8
	github.com/getlantern/proxiedsites v0.0.0-20180805232824-5362487dd72c
	github.com/getlantern/proxy v0.0.0-20190225163220-31d1cc06ed3d
	github.com/getlantern/proxybench v0.0.0-20181017151515-2acfa62efd12
	github.com/getlantern/quicwrapper v0.0.0-20190110220323-f6dd70305d8e
	github.com/getlantern/reconn v0.0.0-20161128113912-7053d017511c // indirect
	github.com/getlantern/ring v0.0.0-20181206150603-dd46ce8faa01 // indirect
	github.com/getlantern/rot13 v0.0.0-20160824200123-33f93fc1fe85
	github.com/getlantern/rotator v0.0.0-20160829164113-013d4f8e36a2
	github.com/getlantern/shortcut v0.0.0-20190117153616-bb4d4203cc25
	github.com/getlantern/stack v0.0.0-20160516001742-02f928aad224 // indirect
	github.com/getlantern/sysproxy v0.0.0-20171129134559-eb982eb14035
	github.com/getlantern/systray v0.0.0-20181206010516-eaad7114094d
	github.com/getlantern/tarfs v0.0.0-20171005185713-4987a6195239
	github.com/getlantern/tinywss v0.0.0-20190711013239-d816e122e1ae
	github.com/getlantern/tlsdefaults v0.0.0-20171004213447-cf35cfd0b1b4
	github.com/getlantern/tlsdialer v0.0.0-20190606180931-1ac26445d532
	github.com/getlantern/unsafeslice v0.0.0-20190520180502-c8f6b3669ee7 // indirect
	github.com/getlantern/uuid v1.2.0
	github.com/getlantern/waitforserver v1.0.1
	github.com/getlantern/wal v0.0.0-20190809100402-c0e8a25ddf75 // indirect
	github.com/getlantern/wfilter v0.0.0-20160829163852-69cc8585ee9c
	github.com/getlantern/winsvc v0.0.0-20160824205134-8bb3a5dbcc1d // indirect
	github.com/getlantern/yaml v0.0.0-20190801163808-0c9bb1ebf426
	github.com/glycerine/goconvey v0.0.0-20190410193231-58a59202ab31 // indirect
	github.com/golang/gddo v0.0.0-20180703174436-daffe1f90ec5 // indirect
	github.com/golang/groupcache v0.0.0-20180513044358-24b0969c4cb7 // indirect
	github.com/gonum/blas v0.0.0-20180125090452-e7c5890b24cf // indirect
	github.com/gonum/floats v0.0.0-20180125090339-7de1f4ea7ab5 // indirect
	github.com/gonum/integrate v0.0.0-20181209220457-a422b5c0fdf2 // indirect
	github.com/gonum/internal v0.0.0-20180125090855-fda53f8d2571 // indirect
	github.com/gonum/lapack v0.0.0-20180125091020-f0b8b25edece // indirect
	github.com/gonum/matrix v0.0.0-20180124231301-a41cc49d4c29 // indirect
	github.com/gonum/stat v0.0.0-20180125090729-ec9c8a1062f4 // indirect
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/google/netstack v0.0.0-20190505230633-4391e4a763ab // indirect
	github.com/google/pprof v0.0.0-20190723021845-34ac40c74b70 // indirect
	github.com/googleapis/gax-go v2.0.0+incompatible // indirect
	github.com/gopherjs/gopherjs v0.0.0-20190430165422-3e4dfb77656c // indirect
	github.com/gorilla/context v1.1.1 // indirect
	github.com/gorilla/mux v1.7.3 // indirect
	github.com/gorilla/websocket v0.0.0-20180306181548-eb925808374e
	github.com/hashicorp/golang-lru v0.5.3
	github.com/huandu/xstrings v1.2.0 // indirect
	github.com/juju/ratelimit v1.0.1 // indirect
	github.com/kardianos/osext v0.0.0-20170510131534-ae77be60afb1
	github.com/keighl/mandrill v0.0.0-20170605120353-1775dd4b3b41
	github.com/kr/binarydist v0.0.0-20160721043806-3035450ff8b9 // indirect
	github.com/kr/pty v1.1.8 // indirect
	github.com/lucas-clemente/quic-go v0.7.1-0.20190207125157-7dc4be2ce994 // indirect
	github.com/mailgun/oxy v0.0.0-20180330141130-3a0f6c4b456b
	github.com/marten-seemann/qtls v0.0.0-20190207043627-591c71538704 // indirect
	github.com/mdlayher/raw v0.0.0-20181016155347-fa5ef3332ca9 // indirect
	github.com/miekg/dns v0.0.0-20180406150955-01d59357d468 // indirect
	github.com/mikioh/tcp v0.0.0-20180707144002-02a37043a4f7 // indirect
	github.com/mikioh/tcpinfo v0.0.0-20180831101334-131b59fef27f // indirect
	github.com/mikioh/tcpopt v0.0.0-20180707144150-7178f18b4ea8 // indirect
	github.com/mitchellh/go-server-timing v0.0.0-20180226015900-d145200e1f90
	github.com/mitchellh/mapstructure v0.0.0-20180220230111-00c29f56e238
	github.com/mitchellh/panicwrap v0.0.0-20170106182340-fce601fe5557
	github.com/openzipkin/zipkin-go v0.1.1 // indirect
	github.com/oschwald/geoip2-golang v1.3.0 // indirect
	github.com/oschwald/maxminddb-golang v1.3.1 // indirect
	github.com/pborman/uuid v0.0.0-20180122190007-c65b2f87fee3
	github.com/petar/GoLLRB v0.0.0-20130427215148-53be0d36a84c // indirect
	github.com/pivotal-cf-experimental/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/prometheus/client_golang v0.9.2 // indirect
	github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90 // indirect
	github.com/prometheus/common v0.4.0 // indirect
	github.com/prometheus/procfs v0.0.0-20190507164030-5867b95ac084 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20180503174638-e2704e165165 // indirect
	github.com/refraction-networking/utls v0.0.0-00010101000000-000000000000
	github.com/skratchdot/open-golang v0.0.0-20160302144031-75fb7ed4208c
	github.com/smartystreets/assertions v0.0.0-20190401211740-f487f9de1cd3 // indirect
	github.com/smartystreets/goconvey v0.0.0-20190330032615-68dc04aab96a // indirect
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.3.0
	github.com/templexxx/cpufeat v0.0.0-20180724012125-cef66df7f161 // indirect
	github.com/templexxx/reedsolomon v0.0.0-20170927015403-7092926d7d05 // indirect
	github.com/templexxx/xor v0.0.0-20170926022130-0af8e873c554 // indirect
	github.com/tevino/abool v0.0.0-20170917061928-9b9efcf221b5
	github.com/tjfoc/gmsm v0.0.0-20180404022540-0effa9db1ba8 // indirect
	github.com/ugorji/go v1.1.7 // indirect
	github.com/vulcand/oxy v0.0.0-20180330141130-3a0f6c4b456b // indirect
	golang.org/x/mobile v0.0.0-20190806162312-597adff16ade // indirect
	golang.org/x/net v0.0.0-20190724013045-ca1201d0de80
	golang.org/x/sys v0.0.0-20190812172437-4e8604ab3aff // indirect
	golang.org/x/tools v0.0.0-20190812171329-f07d81a593a5 // indirect
	gopkg.in/getlantern/yaml.v1 v1.0.0-20140912054538-97d86b60f57e // indirect
	gopkg.in/yaml.v1 v1.0.0-20140924161607-9f9df34309c0 // indirect
	honnef.co/go/tools v0.0.1-2019.2.2 // indirect
)

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.7.1-0.20190606183433-1266fdfeb581

replace github.com/marten-seemann/qtls => github.com/marten-seemann/qtls-deprecated v0.0.0-20190207043627-591c71538704

replace github.com/google/netstack => github.com/getlantern/netstack v0.0.0-20190625160138-4119e572c899

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20190606225154-80c3ccb52074

replace github.com/anacrolix/go-libutp => github.com/getlantern/go-libutp v1.0.3
