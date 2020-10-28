module github.com/getlantern/flashlight

go 1.12

require (
	git.torproject.org/pluggable-transports/goptlib.git v1.0.0
	git.torproject.org/pluggable-transports/obfs4.git v0.0.0-20180421031126-89c21805c212
	github.com/anacrolix/confluence v1.4.0
	github.com/anacrolix/dht/v2 v2.6.1 // indirect
	github.com/anacrolix/envpprof v1.1.1
	github.com/anacrolix/go-libutp v1.0.3
	github.com/anacrolix/log v0.7.1-0.20200604014615-c244de44fd2d
	github.com/anacrolix/tagflag v1.1.1-0.20200411025953-9bb5209d56c2
	github.com/anacrolix/torrent v1.18.1-0.20201028234538-146a16df4ea2
	github.com/blang/semver v0.0.0-20180723201105-3c1074078d32
	github.com/cloudfoundry/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/dchest/siphash v1.2.1 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/fsnotify/fsnotify v1.4.9
	github.com/getlantern/appdir v0.0.0-20200615192800-a0ef1968f4da
	github.com/getlantern/autoupdate v0.0.0-20180719190525-a22eab7ded99
	github.com/getlantern/borda v0.0.0-20200613191039-d7b1c2cc6021
	github.com/getlantern/bufconn v0.0.0-20190625204133-a08544339f8d
	github.com/getlantern/cmux/v2 v2.0.0-20200905031936-c55b16ee8462
	github.com/getlantern/cmuxprivate v0.0.0-20200905032931-afb63438e40b
	github.com/getlantern/common v1.1.1-0.20200824002646-ca4a48d3a74c
	github.com/getlantern/detour v0.0.0-20200814023224-28e20f4ac2d1
	github.com/getlantern/dnsgrab v0.0.0-20191217020031-0e5f714410f9
	github.com/getlantern/domains v0.0.0-20200402172102-34a8db1e0e83
	github.com/getlantern/ema v0.0.0-20190620044903-5943d28f40e4
	github.com/getlantern/enhttp v0.0.0-20190401024120-a974fa851e3c
	github.com/getlantern/enproxy v0.0.0-20180913191734-002212d417a4 // indirect
	github.com/getlantern/errors v1.0.1
	github.com/getlantern/event v0.0.0-20170919023932-f16a5563f52e
	github.com/getlantern/eventual v1.0.0
	github.com/getlantern/filepersist v0.0.0-20160317154340-c5f0cd24e799
	github.com/getlantern/fronted v0.0.0-20200807235213-54b06952ddc6
	github.com/getlantern/geolookup v0.0.0-20200121184643-02217082e50f
	github.com/getlantern/go-ping v0.0.0-20191213124541-9d4b7e6e7de6
	github.com/getlantern/go-socks5 v0.0.0-20171114193258-79d4dd3e2db5
	github.com/getlantern/go-update v0.0.0-20190510022740-79c495ab728c // indirect
	github.com/getlantern/golog v0.0.0-20201022140317-a8d13dc71777
	github.com/getlantern/gowin v0.0.0-20160824205538-88fa116ddffc // indirect
	github.com/getlantern/hellosplitter v0.1.0
	github.com/getlantern/hidden v0.0.0-20190325191715-f02dbb02be55
	github.com/getlantern/http-proxy-lantern/v2 v2.6.23
	github.com/getlantern/httpseverywhere v0.0.0-20190322220559-c364cfbfeb57
	github.com/getlantern/i18n v0.0.0-20181205222232-2afc4f49bb1c
	github.com/getlantern/idletiming v0.0.0-20200228204104-10036786eac5
	github.com/getlantern/ipproxy v0.0.0-20201020142114-ed7e3a8d5d87
	github.com/getlantern/iptool v0.0.0-20170421160045-8723ea29ea42
	github.com/getlantern/jibber_jabber v0.0.0-20160317154340-7346f98d2644
	github.com/getlantern/kcpwrapper v0.0.0-20171114192627-a35c895f6de7
	github.com/getlantern/keyman v0.0.0-20200819205636-76fef27c39f1
	github.com/getlantern/lampshade v0.0.0-20200303040944-fe53f13203e9
	github.com/getlantern/launcher v0.0.0-20160824210503-bc9fc3b11894
	github.com/getlantern/measured v0.0.0-20180919192309-c70b16bb4198
	github.com/getlantern/memhelper v0.0.0-20181113170838-777ea7552231
	github.com/getlantern/meta-scrubber v0.0.1
	github.com/getlantern/mitm v0.0.0-20200517210030-e913809c7038
	github.com/getlantern/mockconn v0.0.0-20191023022503-481dbcceeb58
	github.com/getlantern/mtime v0.0.0-20200417132445-23682092d1f7
	github.com/getlantern/multipath v0.0.0-20201027015000-69ed0bd15259
	github.com/getlantern/netx v0.0.0-20190110220209-9912de6f94fd
	github.com/getlantern/notifier v0.0.0-20190813022016-6b15be83383b
	github.com/getlantern/ops v0.0.0-20200403153110-8476b16edcd6
	github.com/getlantern/osversion v0.0.0-20190510010111-432ecec19031
	github.com/getlantern/packetforward v0.0.0-20200421081927-11933f311913
	github.com/getlantern/profiling v0.0.0-20160317154340-2a15afbadcff
	github.com/getlantern/protected v0.0.0-20190111224713-cc3b5f4a0fb8
	github.com/getlantern/proxy v0.0.0-20200828020017-9c052c8ea590
	github.com/getlantern/proxybench v0.0.0-20200806214955-5d56065f9f77
	github.com/getlantern/psmux v1.5.15-0.20200903210100-947ca5d91683
	github.com/getlantern/quicwrapper v0.0.0-20201013170341-d27d67101f2d
	github.com/getlantern/replica v0.3.1-0.20200623004346-367f62a981a7
	github.com/getlantern/rot13 v0.0.0-20160824200123-33f93fc1fe85
	github.com/getlantern/rotator v0.0.0-20160829164113-013d4f8e36a2
	github.com/getlantern/shortcut v0.0.0-20200404021120-6e9e99fe45a0
	github.com/getlantern/sysproxy v0.0.0-20171129134559-eb982eb14035
	github.com/getlantern/systray v1.0.3-0.20200611154022-031edda14837
	github.com/getlantern/tarfs v0.0.0-20171005185713-4987a6195239
	github.com/getlantern/tinywss v0.0.0-20200121221108-851921f95ad7
	github.com/getlantern/tlsdefaults v0.0.0-20171004213447-cf35cfd0b1b4
	github.com/getlantern/tlsdialer v0.0.0-20200205115148-9bde2ed72c94
	github.com/getlantern/tlsdialer/v3 v3.0.1-0.20200819212748-a098113dd9f3
	github.com/getlantern/tlsmasq v0.3.0
	github.com/getlantern/tlsresumption v0.0.0-20200205020452-74fc6ea4e074
	github.com/getlantern/tlsutil v0.2.0
	github.com/getlantern/uuid v1.2.0
	github.com/getlantern/waitforserver v1.0.1
	github.com/getlantern/winsvc v0.0.0-20160824205134-8bb3a5dbcc1d // indirect
	github.com/getlantern/yaml v0.0.0-20190801163808-0c9bb1ebf426
	github.com/getsentry/sentry-go v0.7.0
	github.com/google/netstack v0.0.0-20191116005144-95bf25ab4723 // indirect
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/golang-lru v0.5.4
	github.com/kardianos/osext v0.0.0-20170510131534-ae77be60afb1
	github.com/keighl/mandrill v0.0.0-20170605120353-1775dd4b3b41
	github.com/kennygrant/sanitize v1.2.4
	github.com/kr/binarydist v0.0.0-20160721043806-3035450ff8b9 // indirect
	github.com/mailgun/oxy v0.0.0-20180330141130-3a0f6c4b456b
	github.com/miekg/dns v0.0.0-20180406150955-01d59357d468
	github.com/mitchellh/go-ps v1.0.0
	github.com/mitchellh/go-server-timing v1.0.0
	github.com/mitchellh/mapstructure v1.1.2
	github.com/mitchellh/panicwrap v1.0.0
	github.com/pborman/uuid v0.0.0-20180122190007-c65b2f87fee3
	github.com/pivotal-cf-experimental/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/refraction-networking/utls v0.0.0-20200729012536-186025ac7b77
	github.com/skratchdot/open-golang v0.0.0-20190402232053-79abb63cd66e
	github.com/stretchr/testify v1.6.1
	github.com/vulcand/oxy v0.0.0-20180330141130-3a0f6c4b456b // indirect
	github.com/xtaci/smux v1.5.15-0.20200704123958-f7188026ba01
	golang.org/x/net v0.0.0-20200904194848-62affa334b73
	golang.org/x/sys v0.0.0-20200929083018-4d22bbb62b3c
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	google.golang.org/appengine v1.6.2 // indirect
	google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55 // indirect
	howett.net/plist v0.0.0-20200419221736-3b63eb3a43b5
)

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.0.0-20201013165432-d264463d99fd

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20200903013459-0c02248f7ce1

replace github.com/anacrolix/go-libutp => github.com/getlantern/go-libutp v1.0.3-0.20190606045409-29ac0bf665ea

// git.apache.org isn't working at the moment, use mirror (should probably switch back once we can)
replace git.apache.org/thrift.git => github.com/apache/thrift v0.0.0-20180902110319-2566ecd5d999

replace github.com/keighl/mandrill => github.com/getlantern/mandrill v0.0.0-20191024010305-7094d8b40358

replace github.com/google/netstack => github.com/getlantern/netstack v0.0.0-20200917193938-4067a7f942d1

//replace github.com/getlantern/replica => ./replica/

// For https://github.com/getlantern/lantern-internal/issues/4277
replace crawshaw.io/sqlite => github.com/getlantern/sqlite v0.3.3-0.20201015053730-228fbc4f607f
