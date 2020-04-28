module github.com/getlantern/flashlight

go 1.12

require (
	git.torproject.org/pluggable-transports/goptlib.git v1.0.0
	git.torproject.org/pluggable-transports/obfs4.git v0.0.0-20180421031126-89c21805c212
	github.com/anacrolix/go-libutp v1.0.1
	github.com/anacrolix/mmsg v1.0.0 // indirect
	github.com/blang/semver v0.0.0-20180723201105-3c1074078d32
	github.com/cloudfoundry/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/dchest/siphash v1.2.1 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/getlantern/appdir v0.0.0-20180320102544-7c0f9d241ea7
	github.com/getlantern/autoupdate v0.0.0-20180719190525-a22eab7ded99
	github.com/getlantern/borda v0.0.0-20200427033127-b36d009c6252
	github.com/getlantern/bufconn v0.0.0-20190625204133-a08544339f8d
	github.com/getlantern/cmux v0.0.0-20200420023238-ddfd0a83b995
	github.com/getlantern/common v0.0.0-20191017131928-864ae49aeb6a
	github.com/getlantern/detour v0.0.0-20191213192126-a4b3dcb2def2
	github.com/getlantern/dnsgrab v0.0.0-20191217020031-0e5f714410f9
	github.com/getlantern/domains v0.0.0-20200402171649-efc86b844fcf
	github.com/getlantern/ema v0.0.0-20190620044903-5943d28f40e4
	github.com/getlantern/enhttp v0.0.0-20190401024120-a974fa851e3c
	github.com/getlantern/enproxy v0.0.0-20180913191734-002212d417a4 // indirect
	github.com/getlantern/errors v0.0.0-20190325191628-abdb3e3e36f7
	github.com/getlantern/event v0.0.0-20170919023932-f16a5563f52e
	github.com/getlantern/eventual v0.0.0-20180125201821-84b02499361b
	github.com/getlantern/filepersist v0.0.0-20160317154340-c5f0cd24e799
	github.com/getlantern/fronted v0.0.0-20190606212108-e7744195eded
	github.com/getlantern/geolookup v0.0.0-20200121184643-02217082e50f
	github.com/getlantern/go-ping v0.0.0-20191213124541-9d4b7e6e7de6
	github.com/getlantern/go-socks5 v0.0.0-20171114193258-79d4dd3e2db5
	github.com/getlantern/go-update v0.0.0-20190510022740-79c495ab728c // indirect
	github.com/getlantern/golog v0.0.0-20190830074920-4ef2e798c2d7
	github.com/getlantern/gowin v0.0.0-20160824205538-88fa116ddffc // indirect
	github.com/getlantern/hidden v0.0.0-20190325191715-f02dbb02be55
	github.com/getlantern/http-proxy-lantern v0.1.4-0.20200130190446-3028d2ef9381
	github.com/getlantern/httpseverywhere v0.0.0-20180326165025-9bdb93e40695
	github.com/getlantern/i18n v0.0.0-20181205222232-2afc4f49bb1c
	github.com/getlantern/idletiming v0.0.0-20190529182719-d2fbc83372a5
	github.com/getlantern/ipproxy v0.0.0-20191216171250-6f1aaa987f2f
	github.com/getlantern/iptool v0.0.0-20170421160045-8723ea29ea42
	github.com/getlantern/jibber_jabber v0.0.0-20160317154340-7346f98d2644
	github.com/getlantern/kcpwrapper v0.0.0-20171114192627-a35c895f6de7
	github.com/getlantern/keyman v0.0.0-20180207174507-f55e7280e93a
	github.com/getlantern/lampshade v0.0.0-20200123165158-e0efbb58c68b
	github.com/getlantern/lantern-server v0.0.0-20200428204212-63fc858a16f5
	github.com/getlantern/launcher v0.0.0-20160824210503-bc9fc3b11894
	github.com/getlantern/measured v0.0.0-20180919192309-c70b16bb4198
	github.com/getlantern/memhelper v0.0.0-20181113170838-777ea7552231
	github.com/getlantern/mitm v0.0.0-20180205214248-4ce456bae650
	github.com/getlantern/mockconn v0.0.0-20191023022503-481dbcceeb58
	github.com/getlantern/mtime v0.0.0-20200417132445-23682092d1f7
	github.com/getlantern/netx v0.0.0-20190110220209-9912de6f94fd
	github.com/getlantern/notifier v0.0.0-20190813022016-6b15be83383b
	github.com/getlantern/ops v0.0.0-20200403153110-8476b16edcd6
	github.com/getlantern/osversion v0.0.0-20190510010111-432ecec19031
	github.com/getlantern/packetforward v0.0.0-20190809094443-386cbcc0d498
	github.com/getlantern/profiling v0.0.0-20160317154340-2a15afbadcff
	github.com/getlantern/protected v0.0.0-20190111224713-cc3b5f4a0fb8
	github.com/getlantern/proxy v0.0.0-20191025190912-b5f45407d9f2
	github.com/getlantern/proxybench v0.0.0-20181017151515-2acfa62efd12
	github.com/getlantern/quic0 v0.0.0-20200121154153-8b18c2ba09f9
	github.com/getlantern/quicwrapper v0.0.0-20200129232925-8ef70253fcae
	github.com/getlantern/rot13 v0.0.0-20160824200123-33f93fc1fe85
	github.com/getlantern/rotator v0.0.0-20160829164113-013d4f8e36a2
	github.com/getlantern/shortcut v0.0.0-20200120121615-2dcb213d447c
	github.com/getlantern/sysproxy v0.0.0-20171129134559-eb982eb14035
	github.com/getlantern/systray v0.0.0-20200324212034-d3ab4fd25d99
	github.com/getlantern/tarfs v0.0.0-20171005185713-4987a6195239
	github.com/getlantern/tinywss v0.0.0-20200121221108-851921f95ad7
	github.com/getlantern/tlsdefaults v0.0.0-20171004213447-cf35cfd0b1b4
	github.com/getlantern/tlsdialer v0.0.0-20200205115148-9bde2ed72c94
	github.com/getlantern/tlsmasq v0.3.0
	github.com/getlantern/tlsresumption v0.0.0-20200205020452-74fc6ea4e074
	github.com/getlantern/uuid v1.2.0
	github.com/getlantern/waitforserver v1.0.1
	github.com/getlantern/winsvc v0.0.0-20160824205134-8bb3a5dbcc1d // indirect
	github.com/getlantern/yaml v0.0.0-20190801163808-0c9bb1ebf426
	github.com/getlantern/yinbi-server v0.0.0-20200428180218-900c16008373
	github.com/getsentry/sentry-go v0.5.1
	github.com/go-chi/chi v4.1.1+incompatible // indirect
	github.com/go-errors/errors v1.0.2 // indirect
	github.com/go-sql-driver/mysql v1.5.0 // indirect
	github.com/gobuffalo/envy v1.9.0 // indirect
	github.com/golang/protobuf v1.4.0 // indirect
	github.com/gorilla/websocket v1.4.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/kardianos/osext v0.0.0-20170510131534-ae77be60afb1
	github.com/keighl/mandrill v0.0.0-20170605120353-1775dd4b3b41
	github.com/konsorten/go-windows-terminal-sequences v1.0.3 // indirect
	github.com/kr/binarydist v0.0.0-20160721043806-3035450ff8b9 // indirect
	github.com/labstack/echo/v4 v4.1.16 // indirect
	github.com/lib/pq v1.4.0 // indirect
	github.com/lxn/walk v0.0.0-20191128110447-55ccb3a9f5c1 // indirect
	github.com/lxn/win v0.0.0-20191128105842-2da648fda5b4 // indirect
	github.com/mailgun/oxy v0.0.0-20180330141130-3a0f6c4b456b
	github.com/miekg/dns v0.0.0-20180406150955-01d59357d468
	github.com/mitchellh/go-server-timing v1.0.0
	github.com/mitchellh/mapstructure v1.1.2
	github.com/mitchellh/panicwrap v0.0.0-20190228164358-f67bf3f3d291
	github.com/pborman/uuid v0.0.0-20180122190007-c65b2f87fee3
	github.com/pivotal-cf-experimental/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/refraction-networking/utls v0.0.0-20190909200633-43c36d3c1f57
	github.com/rs/cors v1.7.0
	github.com/shopspring/decimal v0.0.0-20200428023536-480f653de814 // indirect
	github.com/sirupsen/logrus v1.5.0 // indirect
	github.com/skratchdot/open-golang v0.0.0-20190402232053-79abb63cd66e
	github.com/stellar/go v0.0.0-20200427192452-698593a43c97
	github.com/stellar/go-xdr v0.0.0-20200331223602-71a1e6d555f2 // indirect
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.5.1
	github.com/tyler-smith/go-bip39 v1.0.2 // indirect
	github.com/vulcand/oxy v0.0.0-20180330141130-3a0f6c4b456b // indirect
	golang.org/x/crypto v0.0.0-20200427165652-729f1e841bcc // indirect
	golang.org/x/net v0.0.0-20200425230154-ff2c4b7c35a0
	golang.org/x/sys v0.0.0-20200428200454-593003d681fa // indirect
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/genproto v0.0.0-20200428115010-c45acf45369a // indirect
	google.golang.org/grpc v1.29.1 // indirect
)

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.7.1-0.20200211213545-301421f7c3c9

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20191001223343-79cda44164e3

replace github.com/anacrolix/go-libutp => github.com/getlantern/go-libutp v1.0.3

// git.apache.org isn't working at the moment, use mirror (should probably switch back once we can)
replace git.apache.org/thrift.git => github.com/apache/thrift v0.0.0-20180902110319-2566ecd5d999

replace github.com/keighl/mandrill => github.com/getlantern/mandrill v0.0.0-20191024010305-7094d8b40358

replace github.com/google/netstack => github.com/getlantern/netstack v0.0.0-20191212040217-1650eee50330
