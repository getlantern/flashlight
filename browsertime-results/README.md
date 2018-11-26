# Connection Management Performance Tests
I tested performance of the different Lantern connection management approaches
using [Browsertime](https://www.sitespeed.io/documentation/browsertime/configuration/).

## Summary

- Not preconnecting is always worse than all other options.
- Multiplexing is always at least slightly better than aggressively preconnecting, and on a simulated 3G lossy network connection it even outperforms direct connections.
- Multiplexing also provides the best performance on the video chunk download test.

## Setup

- Tested on Macbook Air
- Computer is on WIFI, 3 feet from router, with Google Fiber internet connection
- Proxy server and website are both running in donyc1, on separate droplets
- Website is a mirror of getlantern.org, served using nginx with a self-signed SSL certificate
- No foreground apps running other than TextEdit and iTerm
- Restarted Lantern before each test run
- Lantern not managing system proxy (so nothing else is proxying through Lantern)
- Proxy All enabled (so Lantern proxies everything it gets)
- Ran 50 iterations (e.g. browsertime --iterations 50 --proxy.http 127.0.0.1:18345 --proxy.https 127.0.0.1:18345 https://159.65.234.90)

## Results (No Throttling)

### Direct (Baseline)

[2018-10-09 10:29:30] INFO: 42 requests, 783.87 kb, backEndTime: 87ms (±4.29ms), firstPaint: 649ms (±9.96ms), DOMContentLoaded: 721ms (±17.32ms), Load: 953ms (±18.13ms), rumSpeedIndex: 776 (±17.04) (50 runs)

### Multiplexed Proxy

[2018-10-09 10:39:39] INFO: 42 requests, 783.90 kb, backEndTime: 76ms (±2.62ms), firstPaint: 804ms (±16.99ms), DOMContentLoaded: 827ms (±19.27ms), Load: 1.14s (±20.11ms), rumSpeedIndex: 806 (±16.93) (50 runs)

### Aggressively Preconnecting Proxy

[2018-10-09 10:59:30] INFO: 42 requests, 783.90 kb, backEndTime: 86ms (±9.52ms), firstPaint: 816ms (±23.37ms), DOMContentLoaded: 846ms (±26.63ms), Load: 1.16s (±29.39ms), rumSpeedIndex: 924 (±26.03) (50 runs)

### Non-preconnecting Proxy

[2018-10-09 10:48:23] INFO: 42 requests, 783.90 kb, backEndTime: 80ms (±3.28ms), firstPaint: 944ms (±11.41ms), DOMContentLoaded: 942ms (±11.21ms), Load: 1.35s (±15.86ms), rumSpeedIndex: 1028 (±12.79) (50 runs)

## Results (With Network Link Conditioner simulating "3G, Lossy Network")

### Direct (Baseline)
[2018-10-09 11:43:58] INFO: 42 requests, 783.93 kb, backEndTime: 95ms (±4.61ms), firstPaint: 662ms (±8.97ms), DOMContentLoaded: 734ms (±14.10ms), Load: 1.01s (±14.30ms), rumSpeedIndex: 816 (±10.96) (50 runs)

### Multiplexed Proxy
[2018-10-09 12:10:52] INFO: 42 requests, 783.90 kb, backEndTime: 67ms (±0.60ms), firstPaint: 771ms (±83.10ms), DOMContentLoaded: 784ms (±82.83ms), Load: 1.05s (±82.03ms), rumSpeedIndex: 772 (±83.07) (50 runs)

### Aggressively Preconnecting Proxy
[2018-10-09 11:50:24] INFO: 42 requests, 783.90 kb, backEndTime: 97ms (±4.19ms), firstPaint: 843ms (±12.38ms), DOMContentLoaded: 873ms (±16.10ms), Load: 1.20s (±20.08ms), rumSpeedIndex: 962 (±15.11) (50 runs)

### Non-preconnecting Proxy
[2018-10-09 11:57:29] INFO: 42 requests, 783.90 kb, backEndTime: 115ms (±12.70ms), firstPaint: 1.06s (±28.09ms), DOMContentLoaded: 1.07s (±33.24ms), Load: 1.54s (±35.33ms), rumSpeedIndex: 1173 (±31.40) (50 runs)

## Video-sized File Download (with Network Link Conditioner simulating "3G, Lossy Network")
This test uses `hey` with a concurrency of 4 (just to stress our concurrent activity handling a little) to download a file similar in size to YouTube content chunks.

`hey -x http://127.0.0.1:18345 -c 4 -n 40 https://159.65.234.90/file.dat`

### Direct (Baseline)

Summary:
  Total:	14.8780 secs
  Slowest:	2.7908 secs
  Fastest:	0.4731 secs
  Average:	1.1731 secs
  Requests/sec:	2.6885

  Total data:	104857600 bytes
  Size/request:	2621440 bytes


### Multiplexed

Summary:
  Total:	8.7241 secs
  Slowest:	2.5123 secs
  Fastest:	0.2692 secs
  Average:	0.8271 secs
  Requests/sec:	4.5850

  Total data:	104857600 bytes
  Size/request:	2621440 bytes

### Aggressive Preconnecting

Summary:
  Total:	12.9325 secs
  Slowest:	7.8560 secs
  Fastest:	0.2705 secs
  Average:	1.0852 secs
  Requests/sec:	3.0930

  Total data:	104857600 bytes
  Size/request:	2621440 bytes

  ### No Preconnecting

  Summary:
    Total:	10.9033 secs
    Slowest:	2.0627 secs
    Fastest:	0.2574 secs
    Average:	0.9054 secs
    Requests/sec:	3.6686

    Total data:	104857600 bytes
    Size/request:	2621440 bytes
