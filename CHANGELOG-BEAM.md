# Changelog

## [Unreleased](https://github.com/getlantern/flashlight/tree/HEAD)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.11...HEAD)

**Merged pull requests:**

- Update anacrolix/torrent to handle more complicated webseed torrent names [\#886](https://github.com/getlantern/flashlight/pull/886) ([max-b](https://github.com/max-b))
- Add a couple small replica+features debugging pieces [\#883](https://github.com/getlantern/flashlight/pull/883) ([max-b](https://github.com/max-b))

## [6.0.11](https://github.com/getlantern/flashlight/tree/6.0.11) (2020-10-02)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.10...6.0.11)

**Merged pull requests:**

- Register debug/conntrack under /replica path to avoid panic on duplic… [\#884](https://github.com/getlantern/flashlight/pull/884) ([oxtoacart](https://github.com/oxtoacart))
- Add basic support for building beam alongside lantern [\#882](https://github.com/getlantern/flashlight/pull/882) ([oxtoacart](https://github.com/oxtoacart))
- Dynamic dnsgrab address [\#881](https://github.com/getlantern/flashlight/pull/881) ([oxtoacart](https://github.com/oxtoacart))
- Fixed compile errors for iOS [\#880](https://github.com/getlantern/flashlight/pull/880) ([oxtoacart](https://github.com/oxtoacart))
- unknow user ID should not belong to any user range when checking for … [\#879](https://github.com/getlantern/flashlight/pull/879) ([joesis](https://github.com/joesis))
- Refactoring replica server handlers for more friendly error reporting/logging/etc [\#876](https://github.com/getlantern/flashlight/pull/876) ([max-b](https://github.com/max-b))
- Supports multipath [\#870](https://github.com/getlantern/flashlight/pull/870) ([joesis](https://github.com/joesis))
- Client hello mimicry [\#855](https://github.com/getlantern/flashlight/pull/855) ([hwh33](https://github.com/hwh33))

## [6.0.10](https://github.com/getlantern/flashlight/tree/6.0.10) (2020-09-19)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.9...6.0.10)

**Merged pull requests:**

- Prevent tls v1.3 client sessions from being cached to deal with utls bug [\#877](https://github.com/getlantern/flashlight/pull/877) ([max-b](https://github.com/max-b))
- Upgraded to latest netstack [\#875](https://github.com/getlantern/flashlight/pull/875) ([oxtoacart](https://github.com/oxtoacart))
- Filter hiddenID from our error Error\(\) method when creating an Exception.Value [\#874](https://github.com/getlantern/flashlight/pull/874) ([max-b](https://github.com/max-b))
- Updated dependencies [\#868](https://github.com/getlantern/flashlight/pull/868) ([myleshorton](https://github.com/myleshorton))
- Fall back to fetching global config directly from GitHub if necessary [\#859](https://github.com/getlantern/flashlight/pull/859) ([oxtoacart](https://github.com/oxtoacart))

## [6.0.9](https://github.com/getlantern/flashlight/tree/6.0.9) (2020-09-11)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.8...6.0.9)

**Merged pull requests:**

- Avoid setting global marketShareData to an empty map [\#873](https://github.com/getlantern/flashlight/pull/873) ([max-b](https://github.com/max-b))

## [6.0.8](https://github.com/getlantern/flashlight/tree/6.0.8) (2020-09-09)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.7...6.0.8)

**Merged pull requests:**

- Fixed simplified Chinese translation [\#872](https://github.com/getlantern/flashlight/pull/872) ([myleshorton](https://github.com/myleshorton))

## [6.0.7](https://github.com/getlantern/flashlight/tree/6.0.7) (2020-09-08)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.6...6.0.7)

**Merged pull requests:**

- Fixed lantern version for sentry [\#871](https://github.com/getlantern/flashlight/pull/871) ([myleshorton](https://github.com/myleshorton))

## [6.0.6](https://github.com/getlantern/flashlight/tree/6.0.6) (2020-09-06)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.5...6.0.6)

**Merged pull requests:**

- Update sentry and other minor ui fixes [\#869](https://github.com/getlantern/flashlight/pull/869) ([myleshorton](https://github.com/myleshorton))
- Report default browser as part of op [\#866](https://github.com/getlantern/flashlight/pull/866) ([hwh33](https://github.com/hwh33))
- Add lantern-linux to hitProxy.py executable options [\#865](https://github.com/getlantern/flashlight/pull/865) ([max-b](https://github.com/max-b))
- Don't propagate panics from stopping Go tun2socks implementation [\#863](https://github.com/getlantern/flashlight/pull/863) ([oxtoacart](https://github.com/oxtoacart))
- Add support for browser-based simulated ClientHellos [\#862](https://github.com/getlantern/flashlight/pull/862) ([hwh33](https://github.com/hwh33))
- Update to latest quicwrapper fixing panic [\#861](https://github.com/getlantern/flashlight/pull/861) ([myleshorton](https://github.com/myleshorton))
- Pull DHT race fix [\#860](https://github.com/getlantern/flashlight/pull/860) ([anacrolix](https://github.com/anacrolix))
- allow configuration of smux or psmux and options [\#858](https://github.com/getlantern/flashlight/pull/858) ([forkner](https://github.com/forkner))
- Tightened up Youtube video URL parsing and added more Youtube domains [\#857](https://github.com/getlantern/flashlight/pull/857) ([oxtoacart](https://github.com/oxtoacart))
- refactor chained package in preparation for supporting multipath [\#856](https://github.com/getlantern/flashlight/pull/856) ([joesis](https://github.com/joesis))
- Add ability to hit different global config for replica [\#848](https://github.com/getlantern/flashlight/pull/848) ([myleshorton](https://github.com/myleshorton))

## [6.0.5](https://github.com/getlantern/flashlight/tree/6.0.5) (2020-08-21)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.4...6.0.5)

**Merged pull requests:**

- not crashing genconfig if gofmt is not installed [\#854](https://github.com/getlantern/flashlight/pull/854) ([joesis](https://github.com/joesis))
- Improve sentry fingerprinting [\#853](https://github.com/getlantern/flashlight/pull/853) ([max-b](https://github.com/max-b))
- Now MITM'ing QQ and 360 safe browsers, closes getlantern/lantern-inte… [\#852](https://github.com/getlantern/flashlight/pull/852) ([oxtoacart](https://github.com/oxtoacart))

## [6.0.4](https://github.com/getlantern/flashlight/tree/6.0.4) (2020-08-13)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.3...6.0.4)

**Merged pull requests:**

- Fixed vertical alignment for cn and translations [\#851](https://github.com/getlantern/flashlight/pull/851) ([myleshorton](https://github.com/myleshorton))

## [6.0.3](https://github.com/getlantern/flashlight/tree/6.0.3) (2020-08-13)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.2...6.0.3)

**Merged pull requests:**

- UI hotfix [\#850](https://github.com/getlantern/flashlight/pull/850) ([myleshorton](https://github.com/myleshorton))

## [6.0.2](https://github.com/getlantern/flashlight/tree/6.0.2) (2020-08-12)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.1...6.0.2)

**Merged pull requests:**

- Updated ui with version display [\#849](https://github.com/getlantern/flashlight/pull/849) ([myleshorton](https://github.com/myleshorton))
- Fix wrong search term in replica analytics reporting [\#847](https://github.com/getlantern/flashlight/pull/847) ([max-b](https://github.com/max-b))

## [6.0.1](https://github.com/getlantern/flashlight/tree/6.0.1) (2020-08-08)

[Full Changelog](https://github.com/getlantern/flashlight/compare/6.0.0...6.0.1)

**Merged pull requests:**

- Adding new UI with surveys and features flags synced [\#846](https://github.com/getlantern/flashlight/pull/846) ([myleshorton](https://github.com/myleshorton))
- Remove replica specific default package version [\#845](https://github.com/getlantern/flashlight/pull/845) ([max-b](https://github.com/max-b))
- Removed unused replica.go [\#844](https://github.com/getlantern/flashlight/pull/844) ([oxtoacart](https://github.com/oxtoacart))
- Add google analytics tracking to replica events [\#843](https://github.com/getlantern/flashlight/pull/843) ([max-b](https://github.com/max-b))

## [6.0.0](https://github.com/getlantern/flashlight/tree/6.0.0) (2020-07-30)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.10.0...6.0.0)

**Merged pull requests:**

- Try longer geolookup test timeout to fix CI failures [\#842](https://github.com/getlantern/flashlight/pull/842) ([max-b](https://github.com/max-b))
- Integrate metascrubber for removing png and jpeg exif/metadata [\#841](https://github.com/getlantern/flashlight/pull/841) ([max-b](https://github.com/max-b))
- Merge devel into replica [\#838](https://github.com/getlantern/flashlight/pull/838) ([max-b](https://github.com/max-b))
- Pull in production UI updates [\#836](https://github.com/getlantern/flashlight/pull/836) ([max-b](https://github.com/max-b))
- Reduce plaintext in Replica [\#834](https://github.com/getlantern/flashlight/pull/834) ([anacrolix](https://github.com/anacrolix))
- Support alternate bucket names [\#831](https://github.com/getlantern/flashlight/pull/831) ([anacrolix](https://github.com/anacrolix))
- Implement proxying and domain fronting for replica-search [\#826](https://github.com/getlantern/flashlight/pull/826) ([max-b](https://github.com/max-b))
- Disable seeding for uploader [\#824](https://github.com/getlantern/flashlight/pull/824) ([max-b](https://github.com/max-b))
- Configure http client for replica s3 connections [\#820](https://github.com/getlantern/flashlight/pull/820) ([myleshorton](https://github.com/myleshorton))
- Second try [\#819](https://github.com/getlantern/flashlight/pull/819) ([bcmertz](https://github.com/bcmertz))
- Allow upload options [\#818](https://github.com/getlantern/flashlight/pull/818) ([bcmertz](https://github.com/bcmertz))
- Remove personally identifiable data from borda submissions [\#817](https://github.com/getlantern/flashlight/pull/817) ([oxtoacart](https://github.com/oxtoacart))
- Explicit s3 torrenting [\#814](https://github.com/getlantern/flashlight/pull/814) ([anacrolix](https://github.com/anacrolix))
- Replica merge from devel [\#812](https://github.com/getlantern/flashlight/pull/812) ([myleshorton](https://github.com/myleshorton))
- Bundle lantern-desktop-ui replica api path fix [\#800](https://github.com/getlantern/flashlight/pull/800) ([max-b](https://github.com/max-b))
- Report replica response metrics to borda [\#799](https://github.com/getlantern/flashlight/pull/799) ([max-b](https://github.com/max-b))
- Enable replica on backend only if enabled in features [\#795](https://github.com/getlantern/flashlight/pull/795) ([myleshorton](https://github.com/myleshorton))
- a little more stateless approach [\#787](https://github.com/getlantern/flashlight/pull/787) ([myleshorton](https://github.com/myleshorton))
- Merge from devel branch [\#786](https://github.com/getlantern/flashlight/pull/786) ([joesis](https://github.com/joesis))
- update enabled features to desktop UI [\#785](https://github.com/getlantern/flashlight/pull/785) ([joesis](https://github.com/joesis))
- Better file name creation [\#784](https://github.com/getlantern/flashlight/pull/784) ([myleshorton](https://github.com/myleshorton))
- Fix replica api url for UI in production [\#783](https://github.com/getlantern/flashlight/pull/783) ([max-b](https://github.com/max-b))

## [5.10.0](https://github.com/getlantern/flashlight/tree/5.10.0) (2020-07-20)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.19...5.10.0)

## [5.9.19](https://github.com/getlantern/flashlight/tree/5.9.19) (2020-07-14)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.18...5.9.19)

## [5.9.18](https://github.com/getlantern/flashlight/tree/5.9.18) (2020-07-09)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.17...5.9.18)

**Merged pull requests:**

- hotfix for quic session deadlock [\#840](https://github.com/getlantern/flashlight/pull/840) ([myleshorton](https://github.com/myleshorton))

## [5.9.17](https://github.com/getlantern/flashlight/tree/5.9.17) (2020-07-08)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.16...5.9.17)

**Merged pull requests:**

- update utls package to include https://github.com/getlantern/utls/pull/4 [\#839](https://github.com/getlantern/flashlight/pull/839) ([joesis](https://github.com/joesis))
- Add replica search to domain fronting lists [\#837](https://github.com/getlantern/flashlight/pull/837) ([max-b](https://github.com/max-b))
- Send keepalive to maintain GA session [\#835](https://github.com/getlantern/flashlight/pull/835) ([joesis](https://github.com/joesis))

## [5.9.16](https://github.com/getlantern/flashlight/tree/5.9.16) (2020-07-02)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.15...5.9.16)

**Merged pull requests:**

- Wait for geolookup before determining whether proxybench is enabled [\#833](https://github.com/getlantern/flashlight/pull/833) ([oxtoacart](https://github.com/oxtoacart))

## [5.9.15](https://github.com/getlantern/flashlight/tree/5.9.15) (2020-06-23)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.14...5.9.15)

## [5.9.14](https://github.com/getlantern/flashlight/tree/5.9.14) (2020-06-15)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.13...5.9.14)

**Merged pull requests:**

- Update to new version of http-proxy-lantern to fix TestProxying [\#830](https://github.com/getlantern/flashlight/pull/830) ([hwh33](https://github.com/hwh33))

## [5.9.13](https://github.com/getlantern/flashlight/tree/5.9.13) (2020-06-11)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.12...5.9.13)

## [5.9.12](https://github.com/getlantern/flashlight/tree/5.9.12) (2020-06-09)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.11...5.9.12)

**Merged pull requests:**

- Fix chromeextension test failure on linux [\#828](https://github.com/getlantern/flashlight/pull/828) ([max-b](https://github.com/max-b))
- Update to latest utls and update a few other repos [\#827](https://github.com/getlantern/flashlight/pull/827) ([myleshorton](https://github.com/myleshorton))

## [5.9.11](https://github.com/getlantern/flashlight/tree/5.9.11) (2020-06-05)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.10...5.9.11)

## [5.9.10](https://github.com/getlantern/flashlight/tree/5.9.10) (2020-06-03)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.9...5.9.10)

**Merged pull requests:**

- Multiplex tlsmasq connections [\#825](https://github.com/getlantern/flashlight/pull/825) ([hwh33](https://github.com/hwh33))

## [5.9.9](https://github.com/getlantern/flashlight/tree/5.9.9) (2020-06-02)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.8...5.9.9)

**Merged pull requests:**

- Update to latest systray with OSX crash workaround [\#829](https://github.com/getlantern/flashlight/pull/829) ([myleshorton](https://github.com/myleshorton))
- update to latest utls with chrome 83 support [\#823](https://github.com/getlantern/flashlight/pull/823) ([myleshorton](https://github.com/myleshorton))
- updates for ip based wss [\#822](https://github.com/getlantern/flashlight/pull/822) ([forkner](https://github.com/forkner))
- Support split ClientHellos [\#821](https://github.com/getlantern/flashlight/pull/821) ([hwh33](https://github.com/hwh33))
- show systray as connecting when starting up [\#816](https://github.com/getlantern/flashlight/pull/816) ([joesis](https://github.com/joesis))
- Drain retry timer only when if it's not consumed [\#815](https://github.com/getlantern/flashlight/pull/815) ([joesis](https://github.com/joesis))
- Enabled YouTube tracking as an optional feature on MacOS [\#810](https://github.com/getlantern/flashlight/pull/810) ([oxtoacart](https://github.com/oxtoacart))

## [5.9.8](https://github.com/getlantern/flashlight/tree/5.9.8) (2020-05-12)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.7...5.9.8)

**Merged pull requests:**

- Don't clear out configs when config is unchanged in cloud [\#811](https://github.com/getlantern/flashlight/pull/811) ([oxtoacart](https://github.com/oxtoacart))
- Include exception 'value' in sentry fingerprint for correct grouping [\#809](https://github.com/getlantern/flashlight/pull/809) ([max-b](https://github.com/max-b))
- report a bit more information when flashlight timeout is hit [\#807](https://github.com/getlantern/flashlight/pull/807) ([joesis](https://github.com/joesis))

## [5.9.7](https://github.com/getlantern/flashlight/tree/5.9.7) (2020-04-29)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.6...5.9.7)

**Merged pull requests:**

- Truncate the message sent to sentry to max allowable chars [\#806](https://github.com/getlantern/flashlight/pull/806) ([max-b](https://github.com/max-b))

## [5.9.6](https://github.com/getlantern/flashlight/tree/5.9.6) (2020-04-27)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.5...5.9.6)

**Merged pull requests:**

- Merge devel into replica [\#808](https://github.com/getlantern/flashlight/pull/808) ([max-b](https://github.com/max-b))
- update to latest borda package [\#805](https://github.com/getlantern/flashlight/pull/805) ([joesis](https://github.com/joesis))
- Actually record pro status when submitting iOS issues [\#804](https://github.com/getlantern/flashlight/pull/804) ([oxtoacart](https://github.com/oxtoacart))
- Exclude 10.10.x.x from Iran shortcut list [\#801](https://github.com/getlantern/flashlight/pull/801) ([joesis](https://github.com/joesis))

## [5.9.5](https://github.com/getlantern/flashlight/tree/5.9.5) (2020-04-24)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.4...5.9.5)

**Merged pull requests:**

- Use more unique fingerprints in an attempt to prevent sentry from grouping distinct panics with the same top level message [\#803](https://github.com/getlantern/flashlight/pull/803) ([max-b](https://github.com/max-b))
- Get UI server from channel [\#802](https://github.com/getlantern/flashlight/pull/802) ([myleshorton](https://github.com/myleshorton))
- Added ability to use hardcoded proxies on iOS [\#797](https://github.com/getlantern/flashlight/pull/797) ([oxtoacart](https://github.com/oxtoacart))

## [5.9.4](https://github.com/getlantern/flashlight/tree/5.9.4) (2020-04-20)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.3...5.9.4)

**Merged pull requests:**

- Use systray with nil check [\#798](https://github.com/getlantern/flashlight/pull/798) ([myleshorton](https://github.com/myleshorton))
- update cmux package for \#3603 [\#796](https://github.com/getlantern/flashlight/pull/796) ([joesis](https://github.com/joesis))

## [5.9.3](https://github.com/getlantern/flashlight/tree/5.9.3) (2020-04-16)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.2...5.9.3)

**Merged pull requests:**

- Added Reconfigure API for iOS [\#794](https://github.com/getlantern/flashlight/pull/794) ([oxtoacart](https://github.com/oxtoacart))
- Reverted to older systray that doesn't use lxn/walk, closes github.co… [\#788](https://github.com/getlantern/flashlight/pull/788) ([oxtoacart](https://github.com/oxtoacart))

## [5.9.2](https://github.com/getlantern/flashlight/tree/5.9.2) (2020-04-15)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.1...5.9.2)

**Merged pull requests:**

- Avoid nil pointer panic and add sentry reporting on app.Exit\(err\) [\#793](https://github.com/getlantern/flashlight/pull/793) ([max-b](https://github.com/max-b))
- Added a new FeatureProxyWhitelistedOnly that causes clients to only p… [\#791](https://github.com/getlantern/flashlight/pull/791) ([oxtoacart](https://github.com/oxtoacart))
- Add duplicate and missing field checks to config checker [\#790](https://github.com/getlantern/flashlight/pull/790) ([hwh33](https://github.com/hwh33))
- Added simple config validator tool [\#789](https://github.com/getlantern/flashlight/pull/789) ([oxtoacart](https://github.com/oxtoacart))

## [5.9.1](https://github.com/getlantern/flashlight/tree/5.9.1) (2020-04-07)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.9.0...5.9.1)

**Merged pull requests:**

- skip re-evaluating dialers when background checking is disabled [\#782](https://github.com/getlantern/flashlight/pull/782) ([joesis](https://github.com/joesis))

## [5.9.0](https://github.com/getlantern/flashlight/tree/5.9.0) (2020-04-03)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.8.6...5.9.0)

## [5.8.6](https://github.com/getlantern/flashlight/tree/5.8.6) (2020-03-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.8.5...5.8.6)

## [5.8.5](https://github.com/getlantern/flashlight/tree/5.8.5) (2020-03-16)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.8.4...5.8.5)

## [5.8.4](https://github.com/getlantern/flashlight/tree/5.8.4) (2020-03-06)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.8.3...5.8.4)

## [5.8.3](https://github.com/getlantern/flashlight/tree/5.8.3) (2020-02-12)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.8.2...5.8.3)

## [5.8.2](https://github.com/getlantern/flashlight/tree/5.8.2) (2020-02-11)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.8.1...5.8.2)

## [5.8.1](https://github.com/getlantern/flashlight/tree/5.8.1) (2020-02-07)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.8.0...5.8.1)

## [5.8.0](https://github.com/getlantern/flashlight/tree/5.8.0) (2020-02-07)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.7.2...5.8.0)

## [5.7.2](https://github.com/getlantern/flashlight/tree/5.7.2) (2019-12-03)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.7.1...5.7.2)

## [5.7.1](https://github.com/getlantern/flashlight/tree/5.7.1) (2019-12-03)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.7.0...5.7.1)

## [5.7.0](https://github.com/getlantern/flashlight/tree/5.7.0) (2019-12-01)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.6.4...5.7.0)

## [5.6.4](https://github.com/getlantern/flashlight/tree/5.6.4) (2019-10-29)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.6.3...5.6.4)

## [5.6.3](https://github.com/getlantern/flashlight/tree/5.6.3) (2019-10-28)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.6.2...5.6.3)

## [5.6.2](https://github.com/getlantern/flashlight/tree/5.6.2) (2019-10-21)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.6.1...5.6.2)

## [5.6.1](https://github.com/getlantern/flashlight/tree/5.6.1) (2019-10-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.6.0...5.6.1)

## [5.6.0](https://github.com/getlantern/flashlight/tree/5.6.0) (2019-10-17)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.8...5.6.0)

## [5.5.8](https://github.com/getlantern/flashlight/tree/5.5.8) (2019-09-24)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.7...5.5.8)

## [5.5.7](https://github.com/getlantern/flashlight/tree/5.5.7) (2019-09-19)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.6...5.5.7)

## [5.5.6](https://github.com/getlantern/flashlight/tree/5.5.6) (2019-08-23)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.5...5.5.6)

## [5.5.5](https://github.com/getlantern/flashlight/tree/5.5.5) (2019-08-23)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.4...5.5.5)

## [5.5.4](https://github.com/getlantern/flashlight/tree/5.5.4) (2019-08-23)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.3...5.5.4)

## [5.5.3](https://github.com/getlantern/flashlight/tree/5.5.3) (2019-08-21)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.2...5.5.3)

## [5.5.2](https://github.com/getlantern/flashlight/tree/5.5.2) (2019-08-20)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.1...5.5.2)

## [5.5.1](https://github.com/getlantern/flashlight/tree/5.5.1) (2019-07-30)

[Full Changelog](https://github.com/getlantern/flashlight/compare/5.5.0...5.5.1)

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

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.2...4.6.4)

## [4.6.2](https://github.com/getlantern/flashlight/tree/4.6.2) (2018-04-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.1...4.6.2)

## [4.6.1](https://github.com/getlantern/flashlight/tree/4.6.1) (2018-04-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.3...4.6.1)

## [4.6.3](https://github.com/getlantern/flashlight/tree/4.6.3) (2018-04-18)

[Full Changelog](https://github.com/getlantern/flashlight/compare/4.6.0...4.6.3)

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
