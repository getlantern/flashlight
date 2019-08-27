# Change Log

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
- Add script to run flashlight on behalf of an user [\#642](https://github.com/getlantern/flashlight/pull/642) ([joesis](https://github.com/joesis))
- Update dependencies to use xtaci/smux v1.3.4 [\#641](https://github.com/getlantern/flashlight/pull/641) ([joesis](https://github.com/joesis))
- Incorporate https://github.com/xtaci/smux/pull/53 [\#640](https://github.com/getlantern/flashlight/pull/640) ([joesis](https://github.com/joesis))

## [5.5.0](https://github.com/getlantern/flashlight/tree/5.5.0) (2019-07-08)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.7...5.5.0)

**Merged pull requests:**

- Fix erroneous checksum for arista [\#639](https://github.com/getlantern/flashlight/pull/639) ([myleshorton](https://github.com/myleshorton))
- Bring in tinywss pull \#12 [\#637](https://github.com/getlantern/flashlight/pull/637) ([joesis](https://github.com/joesis))
- Send iOS error reports straight to Groove [\#635](https://github.com/getlantern/flashlight/pull/635) ([oxtoacart](https://github.com/oxtoacart))
- Added files to iOS report issue test [\#634](https://github.com/getlantern/flashlight/pull/634) ([oxtoacart](https://github.com/oxtoacart))
- go mod dependencies fix [\#633](https://github.com/getlantern/flashlight/pull/633) ([myleshorton](https://github.com/myleshorton))
- Added go tun2socks support [\#563](https://github.com/getlantern/flashlight/pull/563) ([oxtoacart](https://github.com/oxtoacart))

## [5.4.7](https://github.com/getlantern/flashlight/tree/5.4.7) (2019-06-18)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.6...5.4.7)

**Merged pull requests:**

- Disabled compilation of utp on iOS [\#632](https://github.com/getlantern/flashlight/pull/632) ([oxtoacart](https://github.com/oxtoacart))
- Fixed import path for packetforward server in ios/demo, switched to Go modules and updated to latest packetforward [\#619](https://github.com/getlantern/flashlight/pull/619) ([oxtoacart](https://github.com/oxtoacart))

## [5.4.6](https://github.com/getlantern/flashlight/tree/5.4.6) (2019-06-12)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.5...5.4.6)

**Merged pull requests:**

- Disable UTP support in linux [\#631](https://github.com/getlantern/flashlight/pull/631) ([bcmertz](https://github.com/bcmertz))

## [5.4.5](https://github.com/getlantern/flashlight/tree/5.4.5) (2019-06-11)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.4...5.4.5)

**Merged pull requests:**

- disable utp on android [\#630](https://github.com/getlantern/flashlight/pull/630) ([joesis](https://github.com/joesis))

## [5.4.4](https://github.com/getlantern/flashlight/tree/5.4.4) (2019-06-10)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.3...5.4.4)

**Merged pull requests:**

- use master branch of utls fork [\#629](https://github.com/getlantern/flashlight/pull/629) ([forkner](https://github.com/forkner))
- Dynamically sleep based on response from config server [\#624](https://github.com/getlantern/flashlight/pull/624) ([myleshorton](https://github.com/myleshorton))

## [5.4.3](https://github.com/getlantern/flashlight/tree/5.4.3) (2019-06-07)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.2...5.4.3)

**Merged pull requests:**

- add UTP support to Windows XP [\#628](https://github.com/getlantern/flashlight/pull/628) ([joesis](https://github.com/joesis))
- enable domain fronting for wss [\#627](https://github.com/getlantern/flashlight/pull/627) ([forkner](https://github.com/forkner))
- Use Go modules instead of dep [\#616](https://github.com/getlantern/flashlight/pull/616) ([atavism](https://github.com/atavism))

## [5.4.2](https://github.com/getlantern/flashlight/tree/5.4.2) (2019-06-04)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.1...5.4.2)

**Merged pull requests:**

- Remove unused proxy package [\#626](https://github.com/getlantern/flashlight/pull/626) ([myleshorton](https://github.com/myleshorton))
- Added support for using utp in place of tcp [\#609](https://github.com/getlantern/flashlight/pull/609) ([oxtoacart](https://github.com/oxtoacart))

## [5.4.1](https://github.com/getlantern/flashlight/tree/5.4.1) (2019-06-02)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.4.0...5.4.1)

**Merged pull requests:**

- update to latest tinywss which fixes the panic [\#625](https://github.com/getlantern/flashlight/pull/625) ([joesis](https://github.com/joesis))
- Fixed config test [\#623](https://github.com/getlantern/flashlight/pull/623) ([oxtoacart](https://github.com/oxtoacart))
- Disabling failing iOS test [\#622](https://github.com/getlantern/flashlight/pull/622) ([myleshorton](https://github.com/myleshorton))
- Logging for CORS test [\#621](https://github.com/getlantern/flashlight/pull/621) ([myleshorton](https://github.com/myleshorton))
- Update to latest borda [\#620](https://github.com/getlantern/flashlight/pull/620) ([joesis](https://github.com/joesis))
- Support Go 1.11 [\#618](https://github.com/getlantern/flashlight/pull/618) ([hwh33](https://github.com/hwh33))
- Updates to add go tun2socks support [\#617](https://github.com/getlantern/flashlight/pull/617) ([atavism](https://github.com/atavism))
- Support for iOS [\#588](https://github.com/getlantern/flashlight/pull/588) ([oxtoacart](https://github.com/oxtoacart))

## [5.4.0](https://github.com/getlantern/flashlight/tree/5.4.0) (2019-05-10)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.8...5.4.0)

**Merged pull requests:**

- Relatively minor UI tweaks for desktop [\#615](https://github.com/getlantern/flashlight/pull/615) ([myleshorton](https://github.com/myleshorton))
- update go-update again [\#614](https://github.com/getlantern/flashlight/pull/614) ([joesis](https://github.com/joesis))
- Updates go-update to include OS version [\#613](https://github.com/getlantern/flashlight/pull/613) ([myleshorton](https://github.com/myleshorton))
- Adds support for websocket transport [\#612](https://github.com/getlantern/flashlight/pull/612) ([forkner](https://github.com/forkner))
- Using bufconn to make sure that we can always safely read from a conn… [\#611](https://github.com/getlantern/flashlight/pull/611) ([oxtoacart](https://github.com/oxtoacart))
- Avoid potential data loss caused by use of bufio.Reader to read CONNECT response [\#610](https://github.com/getlantern/flashlight/pull/610) ([joesis](https://github.com/joesis))

## [5.3.8](https://github.com/getlantern/flashlight/tree/5.3.8) (2019-04-24)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.7...5.3.8)

**Merged pull requests:**

- Various changes for more prompt connection status [\#608](https://github.com/getlantern/flashlight/pull/608) ([joesis](https://github.com/joesis))
- Resetting whole balancer when connections get protected [\#605](https://github.com/getlantern/flashlight/pull/605) ([oxtoacart](https://github.com/oxtoacart))
- Add beam to track requests end to end [\#598](https://github.com/getlantern/flashlight/pull/598) ([joesis](https://github.com/joesis))

## [5.3.7](https://github.com/getlantern/flashlight/tree/5.3.7) (2019-04-18)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.6...5.3.7)

## [5.3.6](https://github.com/getlantern/flashlight/tree/5.3.6) (2019-04-18)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.5...5.3.6)

**Merged pull requests:**

- Set X-Lantern-Platform as ios for lantern-ios [\#607](https://github.com/getlantern/flashlight/pull/607) ([joesis](https://github.com/joesis))
- don't panic if checking shortcut list times out [\#603](https://github.com/getlantern/flashlight/pull/603) ([joesis](https://github.com/joesis))
- Adds site UAE user suggested and cleans up genconfig [\#599](https://github.com/getlantern/flashlight/pull/599) ([myleshorton](https://github.com/myleshorton))

## [5.3.5](https://github.com/getlantern/flashlight/tree/5.3.5) (2019-04-12)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.4...5.3.5)

**Merged pull requests:**

- new UI for 5.3.5 [\#606](https://github.com/getlantern/flashlight/pull/606) ([myleshorton](https://github.com/myleshorton))
- Get rid of ad blocking altogether [\#604](https://github.com/getlantern/flashlight/pull/604) ([joesis](https://github.com/joesis))

## [5.3.4](https://github.com/getlantern/flashlight/tree/5.3.4) (2019-04-02)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.3...5.3.4)

**Merged pull requests:**

- Flags crash [\#602](https://github.com/getlantern/flashlight/pull/602) ([myleshorton](https://github.com/myleshorton))

## [5.3.3](https://github.com/getlantern/flashlight/tree/5.3.3) (2019-04-01)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.2...5.3.3)

**Merged pull requests:**

- fix the call to Timer.Reset [\#601](https://github.com/getlantern/flashlight/pull/601) ([joesis](https://github.com/joesis))
- Used uuid that ignores touchbar address [\#600](https://github.com/getlantern/flashlight/pull/600) ([myleshorton](https://github.com/myleshorton))
- Use new HTTPS Everywhere test domain that does not redirect itself [\#597](https://github.com/getlantern/flashlight/pull/597) ([myleshorton](https://github.com/myleshorton))
- Ignoring EstRTT in deciding whether we have time to dial proxy [\#596](https://github.com/getlantern/flashlight/pull/596) ([oxtoacart](https://github.com/oxtoacart))
- more logs to debug datacap [\#593](https://github.com/getlantern/flashlight/pull/593) ([myleshorton](https://github.com/myleshorton))

## [5.3.2](https://github.com/getlantern/flashlight/tree/5.3.2) (2019-03-12)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.1...5.3.2)

**Merged pull requests:**

- Don't log individual errors when recording entries to borda [\#595](https://github.com/getlantern/flashlight/pull/595) ([oxtoacart](https://github.com/oxtoacart))
- prevent a QUIC dialer from being null [\#594](https://github.com/getlantern/flashlight/pull/594) ([joesis](https://github.com/joesis))
- cap the time to dial shortcut [\#592](https://github.com/getlantern/flashlight/pull/592) ([joesis](https://github.com/joesis))
- Updated to latest gopacket release for go 1.12 fix [\#591](https://github.com/getlantern/flashlight/pull/591) ([myleshorton](https://github.com/myleshorton))

## [5.3.1](https://github.com/getlantern/flashlight/tree/5.3.1) (2019-02-26)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.3.0...5.3.1)

**Merged pull requests:**

- Set a larger IdleTimeout [\#590](https://github.com/getlantern/flashlight/pull/590) ([joesis](https://github.com/joesis))
- Don't log borda error as error to prevent recursion [\#589](https://github.com/getlantern/flashlight/pull/589) ([joesis](https://github.com/joesis))
- don't mark persistent connections as successful [\#586](https://github.com/getlantern/flashlight/pull/586) ([joesis](https://github.com/joesis))

## [5.3.0](https://github.com/getlantern/flashlight/tree/5.3.0) (2019-02-20)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.8...5.3.0)

**Merged pull requests:**

- Update lampshade to the latest [\#587](https://github.com/getlantern/flashlight/pull/587) ([joesis](https://github.com/joesis))
- add test to cover active probing [\#585](https://github.com/getlantern/flashlight/pull/585) ([joesis](https://github.com/joesis))
- ping 3 times when probing a proxy for connectivity [\#584](https://github.com/getlantern/flashlight/pull/584) ([joesis](https://github.com/joesis))
- remove the misleading 'CONNECT succeeded' log [\#583](https://github.com/getlantern/flashlight/pull/583) ([joesis](https://github.com/joesis))
- update packages to avoid missing commit in qtls dep [\#582](https://github.com/getlantern/flashlight/pull/582) ([forkner](https://github.com/forkner))

## [5.2.8](https://github.com/getlantern/flashlight/tree/5.2.8) (2019-02-01)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.7...5.2.8)

**Merged pull requests:**

- New UI with react 16 and new material UI and accessibility improvements [\#581](https://github.com/getlantern/flashlight/pull/581) ([myleshorton](https://github.com/myleshorton))
- Updated to latest quic-go to fix race condition [\#580](https://github.com/getlantern/flashlight/pull/580) ([oxtoacart](https://github.com/oxtoacart))
- update quic packages to latest [\#579](https://github.com/getlantern/flashlight/pull/579) ([forkner](https://github.com/forkner))
- Add quic integration test [\#577](https://github.com/getlantern/flashlight/pull/577) ([forkner](https://github.com/forkner))
- Update to the latest detour package [\#576](https://github.com/getlantern/flashlight/pull/576) ([joesis](https://github.com/joesis))
- use the latest shortcut package [\#575](https://github.com/getlantern/flashlight/pull/575) ([joesis](https://github.com/joesis))
- update to latest lampshade w/ session pooling [\#572](https://github.com/getlantern/flashlight/pull/572) ([joesis](https://github.com/joesis))
- Added balancer\_dial\_details to capture details of balancer dials [\#564](https://github.com/getlantern/flashlight/pull/564) ([oxtoacart](https://github.com/oxtoacart))

## [5.2.7](https://github.com/getlantern/flashlight/tree/5.2.7) (2019-01-19)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.6...5.2.7)

**Merged pull requests:**

- Fix digest for context [\#574](https://github.com/getlantern/flashlight/pull/574) ([myleshorton](https://github.com/myleshorton))

## [5.2.6](https://github.com/getlantern/flashlight/tree/5.2.6) (2019-01-18)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.5...5.2.6)

**Merged pull requests:**

- fixes for yinbi mouseover and translation [\#573](https://github.com/getlantern/flashlight/pull/573) ([myleshorton](https://github.com/myleshorton))

## [5.2.5](https://github.com/getlantern/flashlight/tree/5.2.5) (2019-01-17)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.4...5.2.5)

**Merged pull requests:**

- dep ensure consistently strips this [\#571](https://github.com/getlantern/flashlight/pull/571) ([myleshorton](https://github.com/myleshorton))
- updated UI for 5.2.5 [\#570](https://github.com/getlantern/flashlight/pull/570) ([myleshorton](https://github.com/myleshorton))
- update to latest context package [\#569](https://github.com/getlantern/flashlight/pull/569) ([joesis](https://github.com/joesis))

## [5.2.4](https://github.com/getlantern/flashlight/tree/5.2.4) (2019-01-08)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.3...5.2.4)

**Merged pull requests:**

- When in development, report everything to borda every 1 minute [\#568](https://github.com/getlantern/flashlight/pull/568) ([oxtoacart](https://github.com/oxtoacart))
- Piping writes to os.Stderr and os.Stdout too [\#567](https://github.com/getlantern/flashlight/pull/567) ([joesis](https://github.com/joesis))

## [5.2.3](https://github.com/getlantern/flashlight/tree/5.2.3) (2019-01-03)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.2...5.2.3)

**Merged pull requests:**

- updated resources for 5.2.3 [\#566](https://github.com/getlantern/flashlight/pull/566) ([myleshorton](https://github.com/myleshorton))
- add email related ports [\#565](https://github.com/getlantern/flashlight/pull/565) ([joesis](https://github.com/joesis))
- Add a test to validate certain basic properties of live global config [\#562](https://github.com/getlantern/flashlight/pull/562) ([forkner](https://github.com/forkner))

## [5.2.2](https://github.com/getlantern/flashlight/tree/5.2.2) (2018-12-13)
[Full Changelog](https://github.com/getlantern/flashlight/compare/5.2.1...5.2.2)

**Merged pull requests:**

- updated UI [\#561](https://github.com/getlantern/flashlight/pull/561) ([myleshorton](https://github.com/myleshorton))

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
[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.11...4.6.13)

## [4.6.11](https://github.com/getlantern/flashlight/tree/4.6.11) (2018-05-01)
[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.12...4.6.11)

## [4.6.12](https://github.com/getlantern/flashlight/tree/4.6.12) (2018-05-01)
[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.10...4.6.12)

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

## [3.7.6](https://github.com/getlantern/flashlight/tree/3.7.6) (2017-07-31)
[Full Changelog](https://github.com/getlantern/flashlight/compare/3.5.12...3.7.6)

## [3.5.12](https://github.com/getlantern/flashlight/tree/3.5.12) (2017-01-02)
[Full Changelog](https://github.com/getlantern/flashlight/compare/0.0.3...3.5.12)

## [0.0.3](https://github.com/getlantern/flashlight/tree/0.0.3) (2014-12-21)
[Full Changelog](https://github.com/getlantern/flashlight/compare/v1.5.12...0.0.3)

## [v1.5.12](https://github.com/getlantern/flashlight/tree/v1.5.12) (2014-11-19)
[Full Changelog](https://github.com/getlantern/flashlight/compare/natty-beta1...v1.5.12)

## [natty-beta1](https://github.com/getlantern/flashlight/tree/natty-beta1) (2014-11-10)
[Full Changelog](https://github.com/getlantern/flashlight/compare/natty-beta2...natty-beta1)

## [natty-beta2](https://github.com/getlantern/flashlight/tree/natty-beta2) (2014-11-10)
[Full Changelog](https://github.com/getlantern/flashlight/compare/natty...natty-beta2)

## [natty](https://github.com/getlantern/flashlight/tree/natty) (2014-11-10)
[Full Changelog](https://github.com/getlantern/flashlight/compare/v1.6.0...natty)

## [v1.6.0](https://github.com/getlantern/flashlight/tree/v1.6.0) (2014-11-10)
[Full Changelog](https://github.com/getlantern/flashlight/compare/v1.5.8...v1.6.0)

## [v1.5.8](https://github.com/getlantern/flashlight/tree/v1.5.8) (2014-11-04)
[Full Changelog](https://github.com/getlantern/flashlight/compare/ipreify-test...v1.5.8)

## [ipreify-test](https://github.com/getlantern/flashlight/tree/ipreify-test) (2014-10-04)
[Full Changelog](https://github.com/getlantern/flashlight/compare/v2.2...ipreify-test)

## [v2.2](https://github.com/getlantern/flashlight/tree/v2.2) (2014-09-27)
[Full Changelog](https://github.com/getlantern/flashlight/compare/v2.1...v2.2)

## [v2.1](https://github.com/getlantern/flashlight/tree/v2.1) (2014-09-27)
[Full Changelog](https://github.com/getlantern/flashlight/compare/v2.0...v2.1)

## [v2.0](https://github.com/getlantern/flashlight/tree/v2.0) (2014-09-27)
[Full Changelog](https://github.com/getlantern/flashlight/compare/docker...v2.0)

## [docker](https://github.com/getlantern/flashlight/tree/docker) (2014-09-27)
[Full Changelog](https://github.com/getlantern/flashlight/compare/vdocker...docker)

## [vdocker](https://github.com/getlantern/flashlight/tree/vdocker) (2014-09-27)
[Full Changelog](https://github.com/getlantern/flashlight/compare/0.0.2...vdocker)

## [0.0.2](https://github.com/getlantern/flashlight/tree/0.0.2) (2014-08-18)
[Full Changelog](https://github.com/getlantern/flashlight/compare/0.0.1...0.0.2)

## [0.0.1](https://github.com/getlantern/flashlight/tree/0.0.1) (2014-07-29)


\* *This Change Log was automatically generated by [github_changelog_generator](https://github.com/skywinder/Github-Changelog-Generator)*