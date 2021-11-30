module github.com/getlantern/flashlight

go 1.16

require (
	git.torproject.org/pluggable-transports/goptlib.git v1.0.0
	git.torproject.org/pluggable-transports/obfs4.git v0.0.0-20180421031126-89c21805c212
	github.com/PuerkitoBio/goquery v1.7.0
	github.com/anacrolix/envpprof v1.1.1 // indirect
	github.com/anacrolix/go-libutp v1.0.4
	github.com/anacrolix/log v0.8.0 // indirect
	github.com/anacrolix/mmsg v1.0.0 // indirect
	github.com/anacrolix/sync v0.2.0 // indirect
	github.com/andybalholm/brotli v1.0.4
	github.com/blang/semver v0.0.0-20180723201105-3c1074078d32
	github.com/dchest/siphash v1.2.1 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/eycorsican/go-tun2socks v1.16.12-0.20201107203946-301549c435ff
	github.com/frankban/quicktest v1.11.3 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/getlantern/appdir v0.0.0-20200615192800-a0ef1968f4da
	github.com/getlantern/bbrconn v0.0.0-20210901194755-12169918fdf9 // indirect
	github.com/getlantern/borda v0.0.0-20211219040702-422f5797af3d
	github.com/getlantern/bufconn v0.0.0-20210901195825-fd7c0267b493
	github.com/getlantern/byteexec v0.0.0-20200509011419-2f5ed5531ada // indirect
	github.com/getlantern/cmux/v2 v2.0.0-20200905031936-c55b16ee8462
	github.com/getlantern/cmuxprivate v0.0.0-20211216020409-d29d0d38be54
	github.com/getlantern/common v1.1.1-0.20211216020417-0ac01e41560d
	github.com/getlantern/detour v0.0.0-20200814023224-28e20f4ac2d1
	github.com/getlantern/dnsgrab v0.0.0-20211216020425-5d5e155a01a8
	github.com/getlantern/domains v0.0.0-20211103190933-f91590174df9
	github.com/getlantern/elevate v0.0.0-20210901195629-ce58359e4d0e // indirect
	github.com/getlantern/ema v0.0.0-20190620044903-5943d28f40e4
	github.com/getlantern/enhttp v0.0.0-20210901195634-6f89d45ee033
	github.com/getlantern/enproxy v0.0.0-20180913191734-002212d417a4 // indirect
	github.com/getlantern/errors v1.0.1
	github.com/getlantern/event v0.0.0-20210901195647-a7e3145142e6
	github.com/getlantern/eventual v1.0.0
	github.com/getlantern/eventual/v2 v2.0.2
	github.com/getlantern/filepersist v0.0.0-20210901195658-ed29a1cb0b7c // indirect
	github.com/getlantern/fronted v0.0.0-20210806163345-971f7e536246
	github.com/getlantern/geolookup v0.0.0-20210901195705-eec711834596
	github.com/getlantern/go-ping v0.0.0-20210901195920-5415d0f18231
	github.com/getlantern/go-socks5 v0.0.0-20171114193258-79d4dd3e2db5
	github.com/getlantern/golog v0.0.0-20210606115803-bce9f9fe5a5f
	github.com/getlantern/grtrack v0.0.0-20210901195719-bdf9e1d12dac // indirect
	github.com/getlantern/hellosplitter v0.1.1
	github.com/getlantern/hidden v0.0.0-20201229170000-e66e7f878730
	github.com/getlantern/http-proxy-lantern/v2 v2.6.50
	github.com/getlantern/httpseverywhere v0.0.0-20201210200013-19ae11fc4eca
	github.com/getlantern/i18n v0.0.0-20181205222232-2afc4f49bb1c
	github.com/getlantern/idletiming v0.0.0-20201229174729-33d04d220c4e
	github.com/getlantern/iptool v0.0.0-20210901195942-5e13a4786de9
	github.com/getlantern/jibber_jabber v0.0.0-20210901195950-68955124cc42
	github.com/getlantern/kcpwrapper v0.0.0-20201001150218-1427e1d39c25
	github.com/getlantern/keepcurrent v0.0.0-20210901200020-9275de720d92 // indirect
	github.com/getlantern/keyman v0.0.0-20210622061955-aa0d47d4932c
	github.com/getlantern/lampshade v0.0.0-20201109225444-b06082e15f3a
	github.com/getlantern/lantern-shadowsocks v1.3.6-0.20210601195915-e04471aa4920
	github.com/getlantern/lantern_aws/salt/update_masquerades v0.0.0-20211130035655-0ef580db2763
	github.com/getlantern/measured v0.0.0-20210507000559-ec5307b2b8be
	github.com/getlantern/mitm v0.0.0-20210622063317-e6510574903b
	github.com/getlantern/mockconn v0.0.0-20200818071412-cb30d065a848
	github.com/getlantern/mtime v0.0.0-20200417132445-23682092d1f7
	github.com/getlantern/multipath v0.0.0-20211105161347-48cd80ec7050
	github.com/getlantern/netx v0.0.0-20211206143627-7ccfeb739cbd
	github.com/getlantern/ops v0.0.0-20200403153110-8476b16edcd6
	github.com/getlantern/osversion v0.0.0-20190510010111-432ecec19031
	github.com/getlantern/pcapper v0.0.0-20210901200029-bf37dc0a4259 // indirect
	github.com/getlantern/probe v0.0.0-20211216020459-69afa01c1c5c // indirect
	github.com/getlantern/probednet v0.0.0-20211216020507-22fd9c1d3bf6 // indirect
	github.com/getlantern/proxy/v2 v2.0.0
	github.com/getlantern/proxybench v0.0.0-20211216020518-199a8fc0d220
	github.com/getlantern/psmux v1.5.15-0.20200903210100-947ca5d91683
	github.com/getlantern/quicwrapper v0.0.0-20211104133553-140f96139f9f
	github.com/getlantern/ring v0.0.0-20210901200052-aea475211e37 // indirect
	github.com/getlantern/rot13 v0.0.0-20210901200056-01bce62cb8bb
	github.com/getlantern/rotator v0.0.0-20160829164113-013d4f8e36a2
	github.com/getlantern/safechannels v0.0.0-20201218194342-b4e5383e9627
	github.com/getlantern/shortcut v0.0.0-20211026183428-bf59a137fdec
	github.com/getlantern/testredis v0.0.0-20210901200107-a4ed71579e17 // indirect
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
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/gopacket v1.1.17
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/hashicorp/golang-lru v0.5.4
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/jaffee/commandeer v0.5.0
	github.com/keighl/mandrill v0.0.0-20170605120353-1775dd4b3b41
	github.com/lucas-clemente/quic-go v0.19.3 // indirect
	github.com/mailgun/oxy v0.0.0-20180330141130-3a0f6c4b456b
	github.com/mitchellh/go-ps v1.0.0
	github.com/mitchellh/go-server-timing v1.0.0
	github.com/mitchellh/mapstructure v1.4.2
	github.com/montanaflynn/stats v0.6.3 // indirect
	github.com/pborman/uuid v1.2.1
	github.com/pivotal-cf-experimental/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/refraction-networking/utls v1.0.0
	github.com/shadowsocks/go-shadowsocks2 v0.1.4-0.20201002022019-75d43273f5a5
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/vulcand/oxy v0.0.0-20180330141130-3a0f6c4b456b // indirect
	github.com/xtaci/smux v1.5.15-0.20200704123958-f7188026ba01
	golang.org/x/net v0.0.0-20211111160137-58aab5ef257a
	golang.org/x/sys v0.0.0-20210809222454-d867a43fc93e
	google.golang.org/genproto v0.0.0-20210406143921-e86de6bf7a46 // indirect
	google.golang.org/grpc v1.37.0 // indirect
	howett.net/plist v0.0.0-20200419221736-3b63eb3a43b5
)

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.0.0-20211103152344-c9ce5bfd4854

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20211116192935-1abdc4b1acab

replace github.com/anacrolix/go-libutp => github.com/getlantern/go-libutp v1.0.3-0.20210202003624-785b5fda134e

// git.apache.org isn't working at the moment, use mirror (should probably switch back once we can)
replace git.apache.org/thrift.git => github.com/apache/thrift v0.0.0-20180902110319-2566ecd5d999

replace github.com/keighl/mandrill => github.com/getlantern/mandrill v0.0.0-20191024010305-7094d8b40358

replace github.com/google/netstack => github.com/getlantern/netstack v0.0.0-20210430190606-84f1a4e5b695

//replace github.com/getlantern/yinbi-server => ../yinbi-server

//replace github.com/getlantern/auth-server => ../auth-server

//replace github.com/getlantern/lantern-server => ../lantern-server

// For https://github.com/crawshaw/sqlite/pull/112 and https://github.com/crawshaw/sqlite/pull/103.
replace crawshaw.io/sqlite => github.com/getlantern/sqlite v0.3.3-0.20210215090556-4f83cf7731f0

replace github.com/eycorsican/go-tun2socks => github.com/getlantern/go-tun2socks v1.16.12-0.20201218023150-b68f09e5ae93

// v0.5.6 has a security issue and using require leaves a reference to it in go.sum
replace github.com/ulikunitz/xz => github.com/ulikunitz/xz v0.5.8
