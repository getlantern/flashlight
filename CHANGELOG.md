# Changelog

## [5.8.1](https://github.com/getlantern/flashlight/tree/5.8.1) (2020-02-07)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.8.0...5.8.1)

**Merged pull requests:**

- disable chrome extension [\#751](https://github.com/getlantern/flashlight/pull/751) ([myleshorton](https://github.com/myleshorton))
- New go mod tidy for latest go stable release [\#750](https://github.com/getlantern/flashlight/pull/750) ([myleshorton](https://github.com/myleshorton))

## [5.8.0](https://github.com/getlantern/flashlight/tree/5.8.0) (2020-02-07)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.7.2...5.8.0)

**Merged pull requests:**

- Added exclude tags [\#749](https://github.com/getlantern/flashlight/pull/749) ([myleshorton](https://github.com/myleshorton))
- Add Sentry panic reporting and adjust panicwrapper signal handling [\#748](https://github.com/getlantern/flashlight/pull/748) ([max-b](https://github.com/max-b))
- Respect server's session tickets when possible even when using config… [\#747](https://github.com/getlantern/flashlight/pull/747) ([oxtoacart](https://github.com/oxtoacart))
- switch back to upstream smux package [\#746](https://github.com/getlantern/flashlight/pull/746) ([joesis](https://github.com/joesis))
- Various iOS fixes [\#744](https://github.com/getlantern/flashlight/pull/744) ([oxtoacart](https://github.com/oxtoacart))
- Added backoff for retrying device linking code validation [\#743](https://github.com/getlantern/flashlight/pull/743) ([oxtoacart](https://github.com/oxtoacart))
- Cleanup Lantern exit for CTRL-C case. [\#742](https://github.com/getlantern/flashlight/pull/742) ([myleshorton](https://github.com/myleshorton))
- update all dependencies to use latest smux [\#741](https://github.com/getlantern/flashlight/pull/741) ([joesis](https://github.com/joesis))
- generate shortcut list from GeoLite2 country database instead [\#739](https://github.com/getlantern/flashlight/pull/739) ([joesis](https://github.com/joesis))
- override shortcut resolver to be able to find real IP of the host on … [\#738](https://github.com/getlantern/flashlight/pull/738) ([joesis](https://github.com/joesis))
- Bring in https://github.com/getlantern/smux/pull/1 [\#737](https://github.com/getlantern/flashlight/pull/737) ([joesis](https://github.com/joesis))
- More iOS pro improvements [\#735](https://github.com/getlantern/flashlight/pull/735) ([oxtoacart](https://github.com/oxtoacart))
- Managing memory usage on iOS more aggressively [\#733](https://github.com/getlantern/flashlight/pull/733) ([oxtoacart](https://github.com/oxtoacart))
- Using more accurate build tags to disable UTP on Linux and iOS [\#732](https://github.com/getlantern/flashlight/pull/732) ([oxtoacart](https://github.com/oxtoacart))
- Added support for pro API to iOS [\#731](https://github.com/getlantern/flashlight/pull/731) ([oxtoacart](https://github.com/oxtoacart))
- Install chrome extension via external installation method [\#730](https://github.com/getlantern/flashlight/pull/730) ([myleshorton](https://github.com/myleshorton))
- support quic ietf draft 24 [\#729](https://github.com/getlantern/flashlight/pull/729) ([forkner](https://github.com/forkner))
- Stores a copy of our settings for the Lantern Chrome Extension to read [\#727](https://github.com/getlantern/flashlight/pull/727) ([myleshorton](https://github.com/myleshorton))
- Prohibit UI from starting on chrome restricted ports [\#726](https://github.com/getlantern/flashlight/pull/726) ([max-b](https://github.com/max-b))
- allow tunneling SSH port by default [\#725](https://github.com/getlantern/flashlight/pull/725) ([joesis](https://github.com/joesis))
- Add GOPRIVATE [\#724](https://github.com/getlantern/flashlight/pull/724) ([myleshorton](https://github.com/myleshorton))
- Add initialize option [\#723](https://github.com/getlantern/flashlight/pull/723) ([joesis](https://github.com/joesis))
- Don't run initial performance probes on iOS to avoid exceeding memory… [\#722](https://github.com/getlantern/flashlight/pull/722) ([oxtoacart](https://github.com/oxtoacart))
- support enabling optional features by various criteria [\#721](https://github.com/getlantern/flashlight/pull/721) ([joesis](https://github.com/joesis))
- Update to latest dnsgrab [\#720](https://github.com/getlantern/flashlight/pull/720) ([oxtoacart](https://github.com/oxtoacart))
- update go-ping to latest to avoid command window on Windows [\#719](https://github.com/getlantern/flashlight/pull/719) ([joesis](https://github.com/joesis))
- Updates to latest detour with close fix [\#718](https://github.com/getlantern/flashlight/pull/718) ([myleshorton](https://github.com/myleshorton))
- bring in systray windows fixes [\#717](https://github.com/getlantern/flashlight/pull/717) ([joesis](https://github.com/joesis))
- Switch to latest netstack and added experimental VPN mode on desktop for tun2socks [\#716](https://github.com/getlantern/flashlight/pull/716) ([oxtoacart](https://github.com/oxtoacart))
- Wss fake sni [\#715](https://github.com/getlantern/flashlight/pull/715) ([forkner](https://github.com/forkner))
- Updated to latest lampshade with client init timestamping [\#714](https://github.com/getlantern/flashlight/pull/714) ([oxtoacart](https://github.com/oxtoacart))

## [5.7.2](https://github.com/getlantern/flashlight/tree/5.7.2) (2019-12-03)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.7.1...5.7.2)

## [5.7.1](https://github.com/getlantern/flashlight/tree/5.7.1) (2019-12-03)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.7.0...5.7.1)

## [5.7.0](https://github.com/getlantern/flashlight/tree/5.7.0) (2019-12-01)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.6.4...5.7.0)

**Merged pull requests:**

- Strip packet capture completely [\#713](https://github.com/getlantern/flashlight/pull/713) ([myleshorton](https://github.com/myleshorton))
- Disable traffic log to compile on linux [\#712](https://github.com/getlantern/flashlight/pull/712) ([myleshorton](https://github.com/myleshorton))
- Using optimized mainloop for windows app [\#711](https://github.com/getlantern/flashlight/pull/711) ([oxtoacart](https://github.com/oxtoacart))
- Added country to issue reports [\#709](https://github.com/getlantern/flashlight/pull/709) ([oxtoacart](https://github.com/oxtoacart))
- Allow forcing config country on Android [\#708](https://github.com/getlantern/flashlight/pull/708) ([oxtoacart](https://github.com/oxtoacart))
- Support borda reporting on iOS [\#707](https://github.com/getlantern/flashlight/pull/707) ([oxtoacart](https://github.com/oxtoacart))
- close hasSucceedingDialer when closing balancer [\#706](https://github.com/getlantern/flashlight/pull/706) ([joesis](https://github.com/joesis))
- Client-side packet capture [\#682](https://github.com/getlantern/flashlight/pull/682) ([hwh33](https://github.com/hwh33))
- Added standalone flag [\#679](https://github.com/getlantern/flashlight/pull/679) ([oxtoacart](https://github.com/oxtoacart))

## [5.6.4](https://github.com/getlantern/flashlight/tree/5.6.4) (2019-10-29)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.6.3...5.6.4)

**Merged pull requests:**

- Fix test failure with older go versions [\#705](https://github.com/getlantern/flashlight/pull/705) ([myleshorton](https://github.com/myleshorton))
- Test for embedded global [\#704](https://github.com/getlantern/flashlight/pull/704) ([myleshorton](https://github.com/myleshorton))
- make sure both proxies and global config are got in config/TestInit [\#703](https://github.com/getlantern/flashlight/pull/703) ([joesis](https://github.com/joesis))
- Improved script and add sanity test for global config updates [\#702](https://github.com/getlantern/flashlight/pull/702) ([myleshorton](https://github.com/myleshorton))

## [5.6.3](https://github.com/getlantern/flashlight/tree/5.6.3) (2019-10-28)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.6.2...5.6.3)

**Merged pull requests:**

- More CI fixes [\#701](https://github.com/getlantern/flashlight/pull/701) ([joesis](https://github.com/joesis))
- Fix Proxy-Connection handling in persistent HTTP case [\#700](https://github.com/getlantern/flashlight/pull/700) ([myleshorton](https://github.com/myleshorton))
- Various changes to make CI more reliable [\#699](https://github.com/getlantern/flashlight/pull/699) ([joesis](https://github.com/joesis))

## [5.6.2](https://github.com/getlantern/flashlight/tree/5.6.2) (2019-10-21)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.6.1...5.6.2)

**Merged pull requests:**

- Fix direct headers [\#698](https://github.com/getlantern/flashlight/pull/698) ([myleshorton](https://github.com/myleshorton))

## [5.6.1](https://github.com/getlantern/flashlight/tree/5.6.1) (2019-10-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.6.0...5.6.1)

**Merged pull requests:**

- Integrating UI that fixes account recovery [\#697](https://github.com/getlantern/flashlight/pull/697) ([myleshorton](https://github.com/myleshorton))
- Different approach to fixing version header issue [\#696](https://github.com/getlantern/flashlight/pull/696) ([oxtoacart](https://github.com/oxtoacart))

## [5.6.0](https://github.com/getlantern/flashlight/tree/5.6.0) (2019-10-17)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.8...5.6.0)

**Merged pull requests:**

- Adding modern-go dependencies -- dependent on go version [\#695](https://github.com/getlantern/flashlight/pull/695) ([myleshorton](https://github.com/myleshorton))
- Server sets trusted [\#694](https://github.com/getlantern/flashlight/pull/694) ([myleshorton](https://github.com/myleshorton))
- A simpler fix for not passing version header to origin sites closes getlantern/lantern-internal\#3047 [\#693](https://github.com/getlantern/flashlight/pull/693) ([oxtoacart](https://github.com/oxtoacart))
- Using commonconfig package for chained config [\#692](https://github.com/getlantern/flashlight/pull/692) ([oxtoacart](https://github.com/oxtoacart))
- Updated to latest proxy [\#691](https://github.com/getlantern/flashlight/pull/691) ([myleshorton](https://github.com/myleshorton))
- Don't reveal Lantern headers when visiting sites directly [\#690](https://github.com/getlantern/flashlight/pull/690) ([joesis](https://github.com/joesis))
- Added support for tls session resumption using pre-negotiated sessions [\#689](https://github.com/getlantern/flashlight/pull/689) ([oxtoacart](https://github.com/oxtoacart))
- Only ping if auto report is enabled and not on Android [\#688](https://github.com/getlantern/flashlight/pull/688) ([oxtoacart](https://github.com/oxtoacart))
- Added stealth mode which disables non-essential network traffic [\#687](https://github.com/getlantern/flashlight/pull/687) ([oxtoacart](https://github.com/oxtoacart))
- Update to latest utls fork [\#684](https://github.com/getlantern/flashlight/pull/684) ([myleshorton](https://github.com/myleshorton))
- allow hitProxy.py to run for multiple proxies [\#681](https://github.com/getlantern/flashlight/pull/681) ([joesis](https://github.com/joesis))
- make QUIC honor tlsservernameindicator config option [\#680](https://github.com/getlantern/flashlight/pull/680) ([forkner](https://github.com/forkner))
- Remove embedded proxies, update global, tweak script [\#676](https://github.com/getlantern/flashlight/pull/676) ([myleshorton](https://github.com/myleshorton))

## [5.5.8](https://github.com/getlantern/flashlight/tree/5.5.8) (2019-09-24)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.7...5.5.8)

**Merged pull requests:**

- print top stacks if the number of goroutines exceeds 800 [\#677](https://github.com/getlantern/flashlight/pull/677) ([joesis](https://github.com/joesis))

## [5.5.7](https://github.com/getlantern/flashlight/tree/5.5.7) (2019-09-19)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.6...5.5.7)

**Merged pull requests:**

- Clients now randomly ping proxies and report stats to borda [\#675](https://github.com/getlantern/flashlight/pull/675) ([oxtoacart](https://github.com/oxtoacart))
- Add timestamp and goroutine dump to integration test [\#674](https://github.com/getlantern/flashlight/pull/674) ([joesis](https://github.com/joesis))
- Using PERCENTILE for reporting dial times [\#673](https://github.com/getlantern/flashlight/pull/673) ([oxtoacart](https://github.com/oxtoacart))
- Fix Go module timestamps for Go 1.13 [\#670](https://github.com/getlantern/flashlight/pull/670) ([anacrolix](https://github.com/anacrolix))
- Update Thrift location [\#668](https://github.com/getlantern/flashlight/pull/668) ([bcmertz](https://github.com/bcmertz))
- Log error if the http proxy port does not behave as expected [\#667](https://github.com/getlantern/flashlight/pull/667) ([joesis](https://github.com/joesis))
- capture all logs generated by child process [\#666](https://github.com/getlantern/flashlight/pull/666) ([joesis](https://github.com/joesis))
- Don't use staging config server [\#663](https://github.com/getlantern/flashlight/pull/663) ([oxtoacart](https://github.com/oxtoacart))
- Support Report Issue Screenshot Upload [\#650](https://github.com/getlantern/flashlight/pull/650) ([bcmertz](https://github.com/bcmertz))

## [5.5.6](https://github.com/getlantern/flashlight/tree/5.5.6) (2019-08-23)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.5...5.5.6)

## [5.5.5](https://github.com/getlantern/flashlight/tree/5.5.5) (2019-08-23)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.4...5.5.5)

## [5.5.4](https://github.com/getlantern/flashlight/tree/5.5.4) (2019-08-23)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.3...5.5.4)

**Merged pull requests:**

- no longer re-create quic dialer when error happens [\#662](https://github.com/getlantern/flashlight/pull/662) ([joesis](https://github.com/joesis))
- update quicwrapper package [\#661](https://github.com/getlantern/flashlight/pull/661) ([forkner](https://github.com/forkner))
- allow certificate validation in addition to pinned certs for wss [\#660](https://github.com/getlantern/flashlight/pull/660) ([forkner](https://github.com/forkner))
- support writing TLS key log [\#658](https://github.com/getlantern/flashlight/pull/658) ([joesis](https://github.com/joesis))
- hitProxy.py: Add the option to force fetching config directly from the proxy [\#651](https://github.com/getlantern/flashlight/pull/651) ([joesis](https://github.com/joesis))

## [5.5.3](https://github.com/getlantern/flashlight/tree/5.5.3) (2019-08-21)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.2...5.5.3)

**Merged pull requests:**

- added lantern free logo fix [\#659](https://github.com/getlantern/flashlight/pull/659) ([myleshorton](https://github.com/myleshorton))
- oquic v0 [\#653](https://github.com/getlantern/flashlight/pull/653) ([forkner](https://github.com/forkner))

## [5.5.2](https://github.com/getlantern/flashlight/tree/5.5.2) (2019-08-20)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.1...5.5.2)

**Merged pull requests:**

- Ping proxies when running diagnostics [\#657](https://github.com/getlantern/flashlight/pull/657) ([hwh33](https://github.com/hwh33))
- iOS - Log proxies.yaml and include in log submission [\#656](https://github.com/getlantern/flashlight/pull/656) ([tomalley104](https://github.com/tomalley104))
- Attach diagnostics when reporting issue [\#655](https://github.com/getlantern/flashlight/pull/655) ([hwh33](https://github.com/hwh33))
- Refactored AdSettings for Tapsell [\#654](https://github.com/getlantern/flashlight/pull/654) ([oxtoacart](https://github.com/oxtoacart))
- Update to use the latest golog [\#652](https://github.com/getlantern/flashlight/pull/652) ([joesis](https://github.com/joesis))
- bumps zip file size limit to ensure all log files are sent [\#649](https://github.com/getlantern/flashlight/pull/649) ([tomalley104](https://github.com/tomalley104))
- Use local cache to avoid redis failure with VPN [\#648](https://github.com/getlantern/flashlight/pull/648) ([myleshorton](https://github.com/myleshorton))

## [5.5.1](https://github.com/getlantern/flashlight/tree/5.5.1) (2019-07-30)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.0...5.5.1)

**Merged pull requests:**

- Assume transports can handle context correctly [\#646](https://github.com/getlantern/flashlight/pull/646) ([joesis](https://github.com/joesis))
- Allow specifying client country when using runAsUser [\#645](https://github.com/getlantern/flashlight/pull/645) ([oxtoacart](https://github.com/oxtoacart))
- Update to latest quicwrapper [\#644](https://github.com/getlantern/flashlight/pull/644) ([joesis](https://github.com/joesis))
- Uniformly report est\_rtt in milliseconds [\#643](https://github.com/getlantern/flashlight/pull/643) ([oxtoacart](https://github.com/oxtoacart))

## [5.5.0](https://github.com/getlantern/flashlight/tree/5.5.0) (2019-07-08)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.7...5.5.0)

## [5.4.7](https://github.com/getlantern/flashlight/tree/5.4.7) (2019-06-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.6...5.4.7)

## [5.4.6](https://github.com/getlantern/flashlight/tree/5.4.6) (2019-06-12)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.5...5.4.6)

## [5.4.5](https://github.com/getlantern/flashlight/tree/5.4.5) (2019-06-11)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.4...5.4.5)

## [5.4.4](https://github.com/getlantern/flashlight/tree/5.4.4) (2019-06-10)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.3...5.4.4)

## [5.4.3](https://github.com/getlantern/flashlight/tree/5.4.3) (2019-06-07)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.2...5.4.3)

## [5.4.2](https://github.com/getlantern/flashlight/tree/5.4.2) (2019-06-04)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.1...5.4.2)

## [5.4.1](https://github.com/getlantern/flashlight/tree/5.4.1) (2019-06-02)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.0...5.4.1)

## [5.4.0](https://github.com/getlantern/flashlight/tree/5.4.0) (2019-05-10)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.8...5.4.0)

## [5.3.8](https://github.com/getlantern/flashlight/tree/5.3.8) (2019-04-24)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.7...5.3.8)

## [5.3.7](https://github.com/getlantern/flashlight/tree/5.3.7) (2019-04-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.6...5.3.7)

## [5.3.6](https://github.com/getlantern/flashlight/tree/5.3.6) (2019-04-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.5...5.3.6)

## [5.3.5](https://github.com/getlantern/flashlight/tree/5.3.5) (2019-04-12)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.4...5.3.5)

## [5.3.4](https://github.com/getlantern/flashlight/tree/5.3.4) (2019-04-02)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.3...5.3.4)

## [5.3.3](https://github.com/getlantern/flashlight/tree/5.3.3) (2019-04-01)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.2...5.3.3)

## [5.3.2](https://github.com/getlantern/flashlight/tree/5.3.2) (2019-03-12)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.1...5.3.2)

## [5.3.1](https://github.com/getlantern/flashlight/tree/5.3.1) (2019-02-26)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.0...5.3.1)

## [5.3.0](https://github.com/getlantern/flashlight/tree/5.3.0) (2019-02-20)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.8...5.3.0)

## [5.2.8](https://github.com/getlantern/flashlight/tree/5.2.8) (2019-02-01)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.7...5.2.8)

## [5.2.7](https://github.com/getlantern/flashlight/tree/5.2.7) (2019-01-19)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.6...5.2.7)

## [5.2.6](https://github.com/getlantern/flashlight/tree/5.2.6) (2019-01-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.5...5.2.6)

## [5.2.5](https://github.com/getlantern/flashlight/tree/5.2.5) (2019-01-17)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.4...5.2.5)

## [5.2.4](https://github.com/getlantern/flashlight/tree/5.2.4) (2019-01-08)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.3...5.2.4)

## [5.2.3](https://github.com/getlantern/flashlight/tree/5.2.3) (2019-01-03)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.2...5.2.3)

## [5.2.2](https://github.com/getlantern/flashlight/tree/5.2.2) (2018-12-13)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.1...5.2.2)

## [5.2.1](https://github.com/getlantern/flashlight/tree/5.2.1) (2018-12-07)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.0...5.2.1)

## [5.2.0](https://github.com/getlantern/flashlight/tree/5.2.0) (2018-12-04)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.1.0...5.2.0)

## [5.1.0](https://github.com/getlantern/flashlight/tree/5.1.0) (2018-12-04)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.0.4...5.1.0)

## [5.0.4](https://github.com/getlantern/flashlight/tree/5.0.4) (2018-12-04)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.0.3...5.0.4)

## [5.0.3](https://github.com/getlantern/flashlight/tree/5.0.3) (2018-12-03)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.0.2...5.0.3)

## [5.0.2](https://github.com/getlantern/flashlight/tree/5.0.2) (2018-12-03)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.0.1...5.0.2)

## [5.0.1](https://github.com/getlantern/flashlight/tree/5.0.1) (2018-12-01)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.0.0...5.0.1)

## [5.0.0](https://github.com/getlantern/flashlight/tree/5.0.0) (2018-11-30)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.9.1-yinbi...5.0.0)

## [4.9.1-yinbi](https://github.com/getlantern/flashlight/tree/4.9.1-yinbi) (2018-11-06)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.9.0...4.9.1-yinbi)

## [4.9.0](https://github.com/getlantern/flashlight/tree/4.9.0) (2018-10-24)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.8.4...4.9.0)

## [4.8.4](https://github.com/getlantern/flashlight/tree/4.8.4) (2018-10-19)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.8.3...4.8.4)

## [4.8.3](https://github.com/getlantern/flashlight/tree/4.8.3) (2018-10-12)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.8.2...4.8.3)

## [4.8.2](https://github.com/getlantern/flashlight/tree/4.8.2) (2018-09-28)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.8.1...4.8.2)

## [4.8.1](https://github.com/getlantern/flashlight/tree/4.8.1) (2018-09-13)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.8.0...4.8.1)

## [4.8.0](https://github.com/getlantern/flashlight/tree/4.8.0) (2018-08-09)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.13...4.8.0)

## [4.7.13](https://github.com/getlantern/flashlight/tree/4.7.13) (2018-08-09)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.12...4.7.13)

## [4.7.12](https://github.com/getlantern/flashlight/tree/4.7.12) (2018-08-01)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.11...4.7.12)

## [4.7.11](https://github.com/getlantern/flashlight/tree/4.7.11) (2018-08-01)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.10...4.7.11)

## [4.7.10](https://github.com/getlantern/flashlight/tree/4.7.10) (2018-07-26)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.9...4.7.10)

## [4.7.9](https://github.com/getlantern/flashlight/tree/4.7.9) (2018-07-11)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.8...4.7.9)

## [4.7.8](https://github.com/getlantern/flashlight/tree/4.7.8) (2018-07-10)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.7...4.7.8)

## [4.7.7](https://github.com/getlantern/flashlight/tree/4.7.7) (2018-07-05)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.6...4.7.7)

## [4.7.6](https://github.com/getlantern/flashlight/tree/4.7.6) (2018-06-21)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.5...4.7.6)

## [4.7.5](https://github.com/getlantern/flashlight/tree/4.7.5) (2018-06-11)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.4...4.7.5)

## [4.7.4](https://github.com/getlantern/flashlight/tree/4.7.4) (2018-06-06)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.3...4.7.4)

## [4.7.3](https://github.com/getlantern/flashlight/tree/4.7.3) (2018-05-31)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.2...4.7.3)

## [4.7.2](https://github.com/getlantern/flashlight/tree/4.7.2) (2018-05-23)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.1...4.7.2)

## [4.7.1](https://github.com/getlantern/flashlight/tree/4.7.1) (2018-05-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.7.0...4.7.1)

## [4.7.0](https://github.com/getlantern/flashlight/tree/4.7.0) (2018-05-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.15...4.7.0)

## [4.6.15](https://github.com/getlantern/flashlight/tree/4.6.15) (2018-05-11)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.14...4.6.15)

## [4.6.14](https://github.com/getlantern/flashlight/tree/4.6.14) (2018-05-06)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.13...4.6.14)

## [4.6.13](https://github.com/getlantern/flashlight/tree/4.6.13) (2018-05-01)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.12...4.6.13)

## [4.6.12](https://github.com/getlantern/flashlight/tree/4.6.12) (2018-05-01)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.11...4.6.12)

## [4.6.11](https://github.com/getlantern/flashlight/tree/4.6.11) (2018-05-01)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.10...4.6.11)

## [4.6.10](https://github.com/getlantern/flashlight/tree/4.6.10) (2018-04-30)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.9...4.6.10)

## [4.6.9](https://github.com/getlantern/flashlight/tree/4.6.9) (2018-04-26)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.8...4.6.9)

## [4.6.8](https://github.com/getlantern/flashlight/tree/4.6.8) (2018-04-26)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.7...4.6.8)

## [4.6.7](https://github.com/getlantern/flashlight/tree/4.6.7) (2018-04-26)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.6...4.6.7)

## [4.6.6](https://github.com/getlantern/flashlight/tree/4.6.6) (2018-04-23)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.4...4.6.6)

## [4.6.4](https://github.com/getlantern/flashlight/tree/4.6.4) (2018-04-20)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.3...4.6.4)

## [4.6.3](https://github.com/getlantern/flashlight/tree/4.6.3) (2018-04-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.2...4.6.3)

## [4.6.2](https://github.com/getlantern/flashlight/tree/4.6.2) (2018-04-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.1...4.6.2)

## [4.6.1](https://github.com/getlantern/flashlight/tree/4.6.1) (2018-04-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.0...4.6.1)

## [4.6.0](https://github.com/getlantern/flashlight/tree/4.6.0) (2018-04-16)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.5.9...4.6.0)

## [4.5.9](https://github.com/getlantern/flashlight/tree/4.5.9) (2018-04-05)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.5.8...4.5.9)

## [4.5.8](https://github.com/getlantern/flashlight/tree/4.5.8) (2018-04-04)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.5.7...4.5.8)

## [4.5.7](https://github.com/getlantern/flashlight/tree/4.5.7) (2018-03-17)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.5.5...4.5.7)

## [4.5.5](https://github.com/getlantern/flashlight/tree/4.5.5) (2018-03-15)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.5.4...4.5.5)

## [4.5.4](https://github.com/getlantern/flashlight/tree/4.5.4) (2018-02-26)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.5.3...4.5.4)

## [4.5.3](https://github.com/getlantern/flashlight/tree/4.5.3) (2018-02-22)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.5.2...4.5.3)

## [4.5.2](https://github.com/getlantern/flashlight/tree/4.5.2) (2018-02-16)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.5.1...4.5.2)

## [4.5.1](https://github.com/getlantern/flashlight/tree/4.5.1) (2018-02-13)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.5.0...4.5.1)

## [4.5.0](https://github.com/getlantern/flashlight/tree/4.5.0) (2018-02-03)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.2.0...4.5.0)

## [4.2.0](https://github.com/getlantern/flashlight/tree/4.2.0) (2017-10-11)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.1.3...4.2.0)

## [4.1.3](https://github.com/getlantern/flashlight/tree/4.1.3) (2017-10-10)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.1.2...4.1.3)

## [4.1.2](https://github.com/getlantern/flashlight/tree/4.1.2) (2017-09-28)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.0.1...4.1.2)

## [4.0.1](https://github.com/getlantern/flashlight/tree/4.0.1) (2017-09-06)

[Full Changelog](https://github.com/getlantern/flashlight/compare/3.7.6...4.0.1)



\* *This Changelog was automatically generated by [github_changelog_generator](https://github.com/github-changelog-generator/github-changelog-generator)*
