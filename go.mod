module github.com/getlantern/flashlight

go 1.15

require (
	git.torproject.org/pluggable-transports/goptlib.git v1.0.0
	git.torproject.org/pluggable-transports/obfs4.git v0.0.0-20180421031126-89c21805c212
	github.com/anacrolix/confluence v1.6.2-0.20201116033747-ba09085bd120
	github.com/anacrolix/envpprof v1.1.1
	github.com/anacrolix/go-libutp v1.0.4
	github.com/anacrolix/log v0.8.0
	github.com/anacrolix/tagflag v1.1.1-0.20200411025953-9bb5209d56c2
	github.com/anacrolix/torrent v1.25.1
	github.com/aws/aws-sdk-go v1.38.3 // indirect
	github.com/blang/semver v0.0.0-20180723201105-3c1074078d32
	github.com/cloudfoundry/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/dchest/siphash v1.2.1 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/eycorsican/go-tun2socks v1.16.12-0.20201107203946-301549c435ff
	github.com/fsnotify/fsnotify v1.4.9
	github.com/getlantern/appdir v0.0.0-20200615192800-a0ef1968f4da
	github.com/getlantern/auth-server v0.0.0-20210323215624-5ae3b1f8a9a4
	github.com/getlantern/autoupdate v0.0.0-20180719190525-a22eab7ded99
	github.com/getlantern/borda v0.0.0-20200613191039-d7b1c2cc6021
	github.com/getlantern/bufconn v0.0.0-20190625204133-a08544339f8d
	github.com/getlantern/cmux/v2 v2.0.0-20200905031936-c55b16ee8462
	github.com/getlantern/cmuxprivate v0.0.0-20200905032931-afb63438e40b
	github.com/getlantern/common v1.1.1-0.20200824002646-ca4a48d3a74c
	github.com/getlantern/detour v0.0.0-20200814023224-28e20f4ac2d1
	github.com/getlantern/diagnostics v0.0.0-20190820054534-b2070abd5177
	github.com/getlantern/dnsgrab v0.0.0-20210120195910-d879cb272122
	github.com/getlantern/domains v0.0.0-20200402172102-34a8db1e0e83
	github.com/getlantern/ema v0.0.0-20190620044903-5943d28f40e4
	github.com/getlantern/enhttp v0.0.0-20190401024120-a974fa851e3c
	github.com/getlantern/enproxy v0.0.0-20180913191734-002212d417a4 // indirect
	github.com/getlantern/errors v1.0.1
	github.com/getlantern/event v0.0.0-20170919023932-f16a5563f52e
	github.com/getlantern/eventual v1.0.0
	github.com/getlantern/filepersist v0.0.0-20160317154340-c5f0cd24e799
	github.com/getlantern/fronted v0.0.0-20201229165541-138879ce456e
	github.com/getlantern/geolookup v0.0.0-20200121184643-02217082e50f
	github.com/getlantern/go-ping v0.0.0-20191213124541-9d4b7e6e7de6
	github.com/getlantern/go-socks5 v0.0.0-20171114193258-79d4dd3e2db5
	github.com/getlantern/go-update v0.0.0-20190510022740-79c495ab728c // indirect
	github.com/getlantern/golog v0.0.0-20201105130739-9586b8bde3a9
	github.com/getlantern/gowin v0.0.0-20160824205538-88fa116ddffc // indirect
	github.com/getlantern/hellosplitter v0.1.0
	github.com/getlantern/hidden v0.0.0-20201229170000-e66e7f878730
	github.com/getlantern/http-proxy-lantern/v2 v2.6.32-0.20210212160215-bbb13b862e06
	github.com/getlantern/httpseverywhere v0.0.0-20201210200013-19ae11fc4eca
	github.com/getlantern/i18n v0.0.0-20181205222232-2afc4f49bb1c
	github.com/getlantern/idletiming v0.0.0-20201229174729-33d04d220c4e
	github.com/getlantern/ipproxy v0.0.0-20201020142114-ed7e3a8d5d87
	github.com/getlantern/iptool v0.0.0-20170421160045-8723ea29ea42
	github.com/getlantern/jibber_jabber v0.0.0-20160317154340-7346f98d2644
	github.com/getlantern/kcpwrapper v0.0.0-20201001150218-1427e1d39c25
	github.com/getlantern/keyman v0.0.0-20200820153608-cfd0ee278507
	github.com/getlantern/lampshade v0.0.0-20201109225444-b06082e15f3a
	github.com/getlantern/lantern-server v0.0.0-20210208142751-07601bac9c9e
	github.com/getlantern/launcher v0.0.0-20160824210503-bc9fc3b11894
	github.com/getlantern/measured v0.0.0-20180919192309-c70b16bb4198
	github.com/getlantern/memhelper v0.0.0-20181113170838-777ea7552231
	github.com/getlantern/meta-scrubber v0.0.1
	github.com/getlantern/mitm v0.0.0-20200517210030-e913809c7038
	github.com/getlantern/mockconn v0.0.0-20200818071412-cb30d065a848
	github.com/getlantern/mtime v0.0.0-20200417132445-23682092d1f7
	github.com/getlantern/multipath v0.0.0-20201027015000-69ed0bd15259
	github.com/getlantern/netx v0.0.0-20201229185957-3fadd2c8f5ba
	github.com/getlantern/notifier v0.0.0-20210109042112-d57e696d0db9
	github.com/getlantern/ops v0.0.0-20200403153110-8476b16edcd6
	github.com/getlantern/osversion v0.0.0-20190510010111-432ecec19031
	github.com/getlantern/packetforward v0.0.0-20201001150407-c68a447b0360
	github.com/getlantern/profiling v0.0.0-20160317154340-2a15afbadcff
	github.com/getlantern/protected v0.0.0-20190111224713-cc3b5f4a0fb8
	github.com/getlantern/proxy v0.0.0-20201001032732-eefd72879266
	github.com/getlantern/proxybench v0.0.0-20200806214955-5d56065f9f77
	github.com/getlantern/psmux v1.5.15-0.20200903210100-947ca5d91683
	github.com/getlantern/quicwrapper v0.0.0-20201013170341-d27d67101f2d
	github.com/getlantern/replica v0.5.1-0.20210425070256-050296eeb8e9
	github.com/getlantern/rot13 v0.0.0-20160824200123-33f93fc1fe85
	github.com/getlantern/rotator v0.0.0-20160829164113-013d4f8e36a2
	github.com/getlantern/safechannels v0.0.0-20201218194342-b4e5383e9627
	github.com/getlantern/shortcut v0.0.0-20200404021120-6e9e99fe45a0
	github.com/getlantern/sysproxy v0.0.0-20171129134559-eb982eb14035
	github.com/getlantern/systray v1.0.3-0.20200611154022-031edda14837
	github.com/getlantern/tarfs v0.0.0-20171005185713-4987a6195239
	github.com/getlantern/timezone v0.0.0-20210104163318-083eaadcecbd
	github.com/getlantern/tinywss v0.0.0-20200121221108-851921f95ad7
	github.com/getlantern/tlsdefaults v0.0.0-20171004213447-cf35cfd0b1b4
	github.com/getlantern/tlsdialer/v3 v3.0.1
	github.com/getlantern/tlsmasq v0.4.2
	github.com/getlantern/tlsresumption v0.0.0-20200205020452-74fc6ea4e074
	github.com/getlantern/tlsutil v0.4.0
	github.com/getlantern/trafficlog v1.0.0
	github.com/getlantern/trafficlog-flashlight v1.0.2
	github.com/getlantern/uuid v1.2.0
	github.com/getlantern/waitforserver v1.0.1
	github.com/getlantern/winsvc v0.0.0-20160824205134-8bb3a5dbcc1d // indirect
	github.com/getlantern/yaml v0.0.0-20190801163808-0c9bb1ebf426
	github.com/getlantern/yinbi-server v0.0.0-20210323195504-45a2d74e067c
	github.com/getsentry/sentry-go v0.9.0
	github.com/go-chi/chi v4.1.2+incompatible
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/golang/mock v1.5.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gopacket v1.1.17
	github.com/google/uuid v1.2.0
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-retryablehttp v0.6.8 // indirect
	github.com/hashicorp/golang-lru v0.5.4
	github.com/jackpal/gateway v1.0.6
	github.com/jinzhu/now v1.1.2 // indirect
	github.com/jmoiron/sqlx v1.3.1 // indirect
	github.com/kardianos/osext v0.0.0-20170510131534-ae77be60afb1
	github.com/keighl/mandrill v0.0.0-20170605120353-1775dd4b3b41
	github.com/kennygrant/sanitize v1.2.4
	github.com/kr/binarydist v0.0.0-20160721043806-3035450ff8b9 // indirect
	github.com/mailgun/oxy v0.0.0-20180330141130-3a0f6c4b456b
	github.com/miekg/dns v1.1.35
	github.com/mitchellh/go-ps v1.0.0
	github.com/mitchellh/go-server-timing v1.0.0
	github.com/mitchellh/mapstructure v1.1.2
	github.com/mitchellh/panicwrap v1.0.0
	github.com/onsi/ginkgo v1.14.2 // indirect
	github.com/onsi/gomega v1.10.3 // indirect
	github.com/pborman/uuid v0.0.0-20180122190007-c65b2f87fee3
	github.com/pivotal-cf-experimental/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/refraction-networking/utls v0.0.0-20200729012536-186025ac7b77
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	github.com/shirou/gopsutil v3.21.2+incompatible // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/skratchdot/open-golang v0.0.0-20190402232053-79abb63cd66e
	github.com/sparrc/go-ping v0.0.0-20190613174326-4e5b6552494c
	github.com/stellar/go v0.0.0-20210323164617-ec5600bd6b2b // indirect
	github.com/stretchr/testify v1.7.0
	github.com/tklauser/go-sysconf v0.3.4 // indirect
	github.com/tyler-smith/go-bip39 v1.1.0 // indirect
	github.com/valyala/fasthttp v1.17.0 // indirect
	github.com/vulcand/oxy v0.0.0-20180330141130-3a0f6c4b456b // indirect
	github.com/xtaci/smux v1.5.15-0.20200704123958-f7188026ba01
	golang.org/x/net v0.0.0-20210323141857-08027d57d8cf
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20210320140829-1e4c9ba3b0c4
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20210323160006-e668133fea6a // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	howett.net/plist v0.0.0-20200419221736-3b63eb3a43b5
)

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.0.0-20201013165432-d264463d99fd

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20200903013459-0c02248f7ce1

replace github.com/anacrolix/go-libutp => github.com/getlantern/go-libutp v1.0.3-0.20210202003624-785b5fda134e

// git.apache.org isn't working at the moment, use mirror (should probably switch back once we can)
replace git.apache.org/thrift.git => github.com/apache/thrift v0.0.0-20180902110319-2566ecd5d999

replace github.com/keighl/mandrill => github.com/getlantern/mandrill v0.0.0-20191024010305-7094d8b40358

replace github.com/google/netstack => github.com/getlantern/netstack v0.0.0-20201204183614-c95cc8f8f085

//replace github.com/getlantern/yinbi-server => ../yinbi-server

//replace github.com/getlantern/auth-server => ../auth-server

//replace github.com/getlantern/lantern-server => ../lantern-server

// For https://github.com/crawshaw/sqlite/pull/112 and https://github.com/crawshaw/sqlite/pull/103.
replace crawshaw.io/sqlite => github.com/getlantern/sqlite v0.3.3-0.20210215090556-4f83cf7731f0

replace github.com/eycorsican/go-tun2socks => github.com/getlantern/go-tun2socks v1.16.12-0.20201218023150-b68f09e5ae93
