module github.com/getlantern/flashlight

go 1.12

require (
	cloud.google.com/go v0.40.0 // indirect
	contrib.go.opencensus.io/exporter/jaeger v0.1.0 // indirect
	git.torproject.org/pluggable-transports/goptlib.git v0.0.0-20180321061416-7d56ec4f381e
	git.torproject.org/pluggable-transports/obfs4.git v0.0.0-20180421031126-89c21805c212
	github.com/anacrolix/go-libutp v1.0.1
	github.com/anacrolix/mmsg v1.0.0 // indirect
	github.com/aristanetworks/goarista v0.0.0-20190628180533-8e7d5b18fe7a // indirect
	github.com/armon/go-radix v0.0.0-20170727155443-1fca145dffbc // indirect
	github.com/beorn7/perks v1.0.0 // indirect
	github.com/bifurcation/mint v0.0.0-20190129141059-83ba9bc2ead9 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/set v0.2.1 // indirect
	github.com/felixge/httpsnoop v1.0.0 // indirect
	github.com/getlantern/appdir v0.0.0-20180320102544-7c0f9d241ea7
	github.com/getlantern/autoupdate v0.0.0-20180719190525-a22eab7ded99
	github.com/getlantern/borda v0.0.0-20190612212110-050956b70e0f
	github.com/getlantern/bufconn v0.0.0-20190503112805-6402607914eb
	github.com/getlantern/cmux v0.0.0-20190603154336-e647fe9f0842
	github.com/getlantern/detour v0.0.0-20190122161414-7107a599a170
	github.com/getlantern/dns v0.0.0-20170920204204-630ccc2c3041 // indirect
	github.com/getlantern/dnsgrab v0.0.0-20171025121014-227f729dcb41
	github.com/getlantern/ema v0.0.0-20180718025023-42474605965c
	github.com/getlantern/enhttp v0.0.0-20190401024120-a974fa851e3c
	github.com/getlantern/enproxy v0.0.0-20180913191734-002212d417a4 // indirect
	github.com/getlantern/errors v0.0.0-20190325191628-abdb3e3e36f7
	github.com/getlantern/event v0.0.0-20170919023932-f16a5563f52e
	github.com/getlantern/eventual v0.0.0-20180125201821-84b02499361b
	github.com/getlantern/filepersist v0.0.0-20160317154340-c5f0cd24e799
	github.com/getlantern/fronted v0.0.0-20190606212108-e7744195eded
	github.com/getlantern/geolookup v0.0.0-20180719190536-68d621f75f46
	github.com/getlantern/go-socks5 v0.0.0-20171114193258-79d4dd3e2db5
	github.com/getlantern/go-update v0.0.0-20190510022740-79c495ab728c // indirect
	github.com/getlantern/golog v0.0.0-20170508214112-cca714f7feb5
	github.com/getlantern/gotun v0.0.0-20190523194503-885514e382d2
	github.com/getlantern/gowin v0.0.0-20160824205538-88fa116ddffc // indirect
	github.com/getlantern/hidden v0.0.0-20190325191715-f02dbb02be55
	github.com/getlantern/http-proxy-lantern v0.1.4-0.20190617184342-d4b587ad290f
	github.com/getlantern/httpseverywhere v0.0.0-20180326165025-9bdb93e40695
	github.com/getlantern/i18n v0.0.0-20181205222232-2afc4f49bb1c
	github.com/getlantern/idletiming v0.0.0-20190529182719-d2fbc83372a5
	github.com/getlantern/iptool v0.0.0-20170421160045-8723ea29ea42
	github.com/getlantern/jibber_jabber v0.0.0-20160317154340-7346f98d2644
	github.com/getlantern/kcpwrapper v0.0.0-20171114192627-a35c895f6de7
	github.com/getlantern/keyman v0.0.0-20180207174507-f55e7280e93a
	github.com/getlantern/lampshade v0.0.0-20190507122828-84b870a67bd6
	github.com/getlantern/launcher v0.0.0-20160824210503-bc9fc3b11894
	github.com/getlantern/measured v0.0.0-20180919192309-c70b16bb4198
	github.com/getlantern/memhelper v0.0.0-20181113170838-777ea7552231
	github.com/getlantern/mitm v0.0.0-20180205214248-4ce456bae650
	github.com/getlantern/mockconn v0.0.0-20190403061815-a8ffa60494a6
	github.com/getlantern/mtime v0.0.0-20170117193331-ba114e4a82b0
	github.com/getlantern/netx v0.0.0-20190110220209-9912de6f94fd
	github.com/getlantern/notifier v0.0.0-20190531021800-64e5c4112f43
	github.com/getlantern/ops v0.0.0-20190325191751-d70cb0d6f85f
	github.com/getlantern/osversion v0.0.0-20180309120706-8f3fb296110c
	github.com/getlantern/packetforward v0.0.0-20190617161814-583f227df593
	github.com/getlantern/profiling v0.0.0-20160317154340-2a15afbadcff
	github.com/getlantern/protected v0.0.0-20190111224713-cc3b5f4a0fb8
	github.com/getlantern/proxiedsites v0.0.0-20180805232824-5362487dd72c
	github.com/getlantern/proxy v0.0.0-20190225163220-31d1cc06ed3d
	github.com/getlantern/proxybench v0.0.0-20181017151515-2acfa62efd12
	github.com/getlantern/quic-go v0.9.0 // indirect
	github.com/getlantern/quicwrapper v0.0.0-20190110220323-f6dd70305d8e
	github.com/getlantern/rot13 v0.0.0-20160824200123-33f93fc1fe85
	github.com/getlantern/rotator v0.0.0-20160829164113-013d4f8e36a2
	github.com/getlantern/shortcut v0.0.0-20190117153616-bb4d4203cc25
	github.com/getlantern/sysproxy v0.0.0-20171129134559-eb982eb14035
	github.com/getlantern/systray v0.0.0-20181206010516-eaad7114094d
	github.com/getlantern/tarfs v0.0.0-20171005185713-4987a6195239
	github.com/getlantern/tinywss v0.0.0-20190603141034-49fb977700a3
	github.com/getlantern/tlsdefaults v0.0.0-20171004213447-cf35cfd0b1b4
	github.com/getlantern/tlsdialer v0.0.0-20190606180931-1ac26445d532
	github.com/getlantern/uuid v1.2.0
	github.com/getlantern/waitforserver v1.0.1
	github.com/getlantern/wfilter v0.0.0-20160829163852-69cc8585ee9c
	github.com/getlantern/winsvc v0.0.0-20160824205134-8bb3a5dbcc1d // indirect
	github.com/getlantern/yaml v0.0.0-20160317154340-79303eb9c0d9
	github.com/getlantern/zenodb v0.0.0-20190618200703-0869a10e3c9c // indirect
	github.com/go-ole/go-ole v1.2.1 // indirect
	github.com/gogo/protobuf v1.2.0 // indirect
	github.com/golang/gddo v0.0.0-20180703174436-daffe1f90ec5 // indirect
	github.com/google/pprof v0.0.0-20190515194954-54271f7e092f // indirect
	github.com/googleapis/gax-go/v2 v2.0.5 // indirect
	github.com/gorilla/mux v1.7.2 // indirect
	github.com/gorilla/websocket v0.0.0-20180306181548-eb925808374e
	github.com/hashicorp/golang-lru v0.5.1
	github.com/kardianos/osext v0.0.0-20170510131534-ae77be60afb1
	github.com/keighl/mandrill v0.0.0-20170605120353-1775dd4b3b41
	github.com/kr/binarydist v0.0.0-20160721043806-3035450ff8b9 // indirect
	github.com/mailgun/oxy v0.0.0-20180330141130-3a0f6c4b456b
	github.com/miekg/dns v0.0.0-20180406150955-01d59357d468 // indirect
	github.com/mitchellh/go-server-timing v0.0.0-20180226015900-d145200e1f90
	github.com/mitchellh/mapstructure v0.0.0-20180220230111-00c29f56e238
	github.com/mitchellh/panicwrap v0.0.0-20170106182340-fce601fe5557
	github.com/opentracing/opentracing-go v1.1.0 // indirect
	github.com/oschwald/geoip2-golang v1.3.0 // indirect
	github.com/oschwald/maxminddb-golang v1.3.1 // indirect
	github.com/pborman/uuid v0.0.0-20180122190007-c65b2f87fee3
	github.com/prometheus/client_golang v0.9.3-0.20190127221311-3c4408c8b829 // indirect
	github.com/prometheus/common v0.4.0 // indirect
	github.com/prometheus/procfs v0.0.0-20190507164030-5867b95ac084 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20181016184325-3113b8401b8a // indirect
	github.com/refraction-networking/utls v0.0.0-00010101000000-000000000000
	github.com/skratchdot/open-golang v0.0.0-20160302144031-75fb7ed4208c
	github.com/stretchr/testify v1.3.0
	github.com/tevino/abool v0.0.0-20170917061928-9b9efcf221b5
	github.com/tjfoc/gmsm v0.0.0-20180404022540-0effa9db1ba8 // indirect
	github.com/uber/jaeger-client-go v2.16.0+incompatible // indirect
	github.com/uber/jaeger-lib v2.0.0+incompatible // indirect
	github.com/vulcand/oxy v0.0.0-20180330141130-3a0f6c4b456b // indirect
	github.com/xtaci/smux v1.0.7 // indirect
	go.opencensus.io v0.22.0 // indirect
	golang.org/x/crypto v0.0.0-20190621222207-cc06ce4a13d4 // indirect
	golang.org/x/exp v0.0.0-20190627132806-fd42eb6b336f // indirect
	golang.org/x/image v0.0.0-20190622003408-7e034cad6442 // indirect
	golang.org/x/mobile v0.0.0-20190607214518-6fa95d984e88 // indirect
	golang.org/x/net v0.0.0-20190628185345-da137c7871d7
	golang.org/x/sys v0.0.0-20190626221950-04f50cda93cb // indirect
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect
	golang.org/x/tools v0.0.0-20190628203336-59bec042292d // indirect
	google.golang.org/api v0.7.0 // indirect
	google.golang.org/appengine v1.6.1 // indirect
	google.golang.org/genproto v0.0.0-20190627203621-eb59cef1c072 // indirect
	google.golang.org/grpc v1.21.1 // indirect
)

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.7.1-0.20190606183433-1266fdfeb581

replace github.com/marten-seemann/qtls => github.com/marten-seemann/qtls-deprecated v0.0.0-20190207043627-591c71538704

replace github.com/google/netstack => github.com/getlantern/netstack v0.0.0-20190314012628-8999826b821d

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20190606225154-80c3ccb52074

replace github.com/anacrolix/go-libutp => github.com/getlantern/go-libutp v1.0.3
