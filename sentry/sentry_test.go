package sentry

import (
	"testing"

	sentrySDK "github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
)

type panicTest struct {
	panicMessage   string
	expectedOutput string
}

func TestSentryPanicMessage(t *testing.T) {
	var panicTests = []panicTest{
		{
			panicMessage: `panic: Failed to load ws2_32.dll: The specified module could not be found.

goroutine 89 [running]:
syscall.(*LazyProc).mustFind(0x13d0d700)
	/usr/local/go/src/syscall/dll_windows.go:311 +0x42
syscall.(*LazyProc).Addr(0x13d0d700, 0x3828122)
	/usr/local/go/src/syscall/dll_windows.go:318 +0x21
internal/syscall/windows.WSASocket(0x2, 0x1, 0x0, 0x0, 0x0, 0x81, 0x40b9af, 0xffffffff, 0x1fffff)
	/usr/local/go/src/internal/syscall/windows/zsyscall_windows.go:122 +0x27
net.sysSocket(0x2, 0x1, 0x0, 0x5787fb, 0x13c24230, 0x1adb0f0)
	/usr/local/go/src/net/sock_windows.go:20 +0x52`,
			expectedOutput: `panic: Failed to load ws2_32.dll: The specified module could not be found.
/usr/local/go/src/syscall/dll_windows.go:311
/usr/local/go/src/syscall/dll_windows.go:318
/usr/local/go/src/internal/syscall/windows/zsyscall_windows.go:122
/usr/local/go/src/net/sock_windows.go:20`,
		},
		{
			panicMessage: `fatal error: fault
[signal 0xc0000005 code=0x0 addr=0xe0c18df1 pc=0x42d87d]

goroutine 50143 [running]:
runtime.throw(0x127137a, 0x5)
	/usr/local/go/src/runtime/panic.go:1116 +0x64 fp=0x16eafd38 sp=0x16eafd24 pc=0x433fb4
runtime.sigpanic()
	/usr/local/go/src/runtime/signal_windows.go:249 +0x1ed fp=0x16eafd4c sp=0x16eafd38 pc=0x44806d
internal/poll.runtime_pollSetDeadline(0xe0c18df1, 0x0, 0x80000000, 0x77)
	/usr/local/go/src/runtime/netpoll.go:225 +0x1d fp=0x16eafda4 sp=0x16eafd4c pc=0x42d87d
internal/poll.setDeadlineImpl(0x173c65c0, 0xe800, 0x0, 0x3a00, 0x0, 0xa800, 0x77, 0x0, 0x0)
	/usr/local/go/src/internal/poll/fd_poll_runtime.go:155 +0x146 fp=0x16eafdec sp=0x16eafda4 pc=0x4c6d46
internal/poll.(*FD).SetWriteDeadline(...)
	/usr/local/go/src/internal/poll/fd_poll_runtime.go:137
net.(*netFD).SetWriteDeadline(...)
	/usr/local/go/src/net/fd_windows.go:255
net.(*conn).SetWriteDeadline(0x16f796b0, 0xe800, 0x0, 0x3a00, 0x0, 0xa800, 0x0, 0x0)
	/usr/local/go/src/net/net.go:262 +0x71 fp=0x16eafe34 sp=0x16eafdec pc=0x583a61
github.com/getlantern/flashlight/vendor/github.com/getlantern/idletiming.(*IdleTimingConn).doWrite(0x1717a840, 0x17026c00, 0x5a3, 0x5a8, 0x5a8, 0x5a3, 0x0)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/idletiming/idletiming_conn.go:191 +0x400 fp=0x16eafef4 sp=0x16eafe34 pc=0x7e96e0
github.com/getlantern/flashlight/vendor/github.com/getlantern/idletiming.(*IdleTimingConn).Write(0x1717a840, 0x17026c00, 0x5a3, 0x5a8, 0x5a3, 0x0, 0x0)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/idletiming/idletiming_conn.go:171 +0x51 fp=0x16eaff24 sp=0x16eafef4 pc=0x7e9291
github.com/getlantern/flashlight/vendor/github.com/getlantern/netx.doCopy(0x15064c0, 0x1717a840, 0x1506a40, 0x17400508, 0x17026c00, 0x5a3, 0x5a8, 0x1699d1c0, 0x1756db98)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/netx/copy.go:50 +0x327 fp=0x16eaffc8 sp=0x16eaff24 pc=0x7e7fa7
runtime.goexit()
	/usr/local/go/src/runtime/asm_386.s:1337 +0x1 fp=0x16eaffcc sp=0x16eaffc8 pc=0x4607e1
created by github.com/getlantern/flashlight/vendor/github.com/getlantern/netx.BidiCopy
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/netx/copy.go:23 +0x127`,
			expectedOutput: `fatal error: fault
/usr/local/go/src/runtime/panic.go:1116
/usr/local/go/src/runtime/signal_windows.go:249
/usr/local/go/src/runtime/netpoll.go:225
/usr/local/go/src/internal/poll/fd_poll_runtime.go:155
/usr/local/go/src/internal/poll/fd_poll_runtime.go:137
/usr/local/go/src/net/fd_windows.go:255
/usr/local/go/src/net/net.go:262
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/idletiming/idletiming_conn.go:191
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/idletiming/idletiming_conn.go:171
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/netx/copy.go:50
/usr/local/go/src/runtime/asm_386.s:1337
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/netx/copy.go:23`,
		},
		{
			panicMessage: `panic: Unable to set systray icon: Shell_NotifyIcon

Stack:
goroutine 35 [running]:
runtime/debug.Stack(0x2, 0x1, 0x18713a38)
	/usr/local/go/src/runtime/debug/stack.go:24 +0x83
github.com/getlantern/flashlight/vendor/github.com/lxn/walk.newErr(...)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lxn/walk/error.go:81
github.com/getlantern/flashlight/vendor/github.com/lxn/walk.newError(0x1380c4c, 0x10, 0x1611500, 0x158fd520)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lxn/walk/error.go:85 +0x1e
github.com/getlantern/flashlight/vendor/github.com/lxn/walk.(*NotifyIcon).SetIcon(0x147f6050, 0x1611520, 0x158fd520, 0x158fd520, 0x0)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lxn/walk/notifyicon.go:284 +0x109
github.com/getlantern/flashlight/vendor/github.com/getlantern/systray.SetIcon(0x13b538b, 0x47e, 0x47e)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/systray/systray_windows.go:97 +0x1a2
main.statsUpdated()
	/lantern/src/github.com/getlantern/flashlight/main/systray.go:173 +0xd8
main.configureSystemTray.func1(0x168f26c0, 0x9, 0x168f26d8, 0x5, 0x168f2708, 0x2, 0x0, 0x0, 0x1000100, 0x1, ...)
	/lantern/src/github.com/getlantern/flashlight/main/systray.go:105 +0x4f
github.com/getlantern/flashlight/stats.(*tracker).AddListener.func1(0x1320460, 0x16d354c0)
	/lantern/src/github.com/getlantern/flashlight/stats/stats_tracker.go:135 +0x50
github.com/getlantern/flashlight/vendor/github.com/getlantern/event.(*listener).acceptLoop(0x144d80f0)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/event/event.go:100 +0x5c
created by github.com/getlantern/flashlight/vendor/github.com/getlantern/event.(*dispatcher).AddListener
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/event/event.go:56 +0xc1`,
			expectedOutput: `panic: Unable to set systray icon: Shell_NotifyIcon
/usr/local/go/src/runtime/debug/stack.go:24
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lxn/walk/error.go:81
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lxn/walk/error.go:85
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lxn/walk/notifyicon.go:284
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/systray/systray_windows.go:97
/lantern/src/github.com/getlantern/flashlight/main/systray.go:173
/lantern/src/github.com/getlantern/flashlight/main/systray.go:105
/lantern/src/github.com/getlantern/flashlight/stats/stats_tracker.go:135
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/event/event.go:100
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/event/event.go:56`,
		},
		{
			panicMessage: `panic: (runtime.plainError) (0x11b43e0,0x14e8588)
fatal error: panic on system stack

runtime stack:
time.sendTime(0x114dd60, 0x19117540, 0x0)
	/usr/local/go/src/time/sleep.go:137 +0x47

goroutine 1 [syscall, 2 minutes, locked to thread]:
syscall.Syscall6(0x7565e7a0, 0x4, 0x14550080, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, ...)
	/usr/local/go/src/runtime/syscall_windows.go:201 +0xbb
github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows.(*Proc).Call(0x1457c100, 0x18447530, 0x4, 0x4, 0x10, 0x115ed60, 0x14f8101, 0x18447530)
	/lantern/src/github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows/dll_windows.go:174 +0x28b
github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows.(*LazyProc).Call(0x148219e0, 0x18447530, 0x4, 0x4, 0x0, 0x0, 0x14f8180, 0x1add9c0)
	/lantern/src/github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows/dll_windows.go:346 +0x48
github.com/getlantern/flashlight/vendor/github.com/getlantern/systray.nativeLoop()
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/systray/systray_windows.go:742 +0x10f
github.com/getlantern/flashlight/vendor/github.com/getlantern/systray.Run(0x1448d890, 0x14493860)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/systray/systray.go:75 +0x2e
github.com/getlantern/flashlight/desktop.RunOnSystrayReady(0x11a7500, 0x1507200, 0x1461c0c0, 0x1448d890)
	/lantern/src/github.com/getlantern/flashlight/desktop/systray.go:60 +0x64
main.main()
	/lantern/src/github.com/getlantern/flashlight/main/main.go:152 +0x299

goroutine 35 [chan receive, 9 minutes]:
github.com/getlantern/flashlight/vendor/github.com/getlantern/event.(*dispatcher).dispatchLoop(0x1488c1e0)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/event/event.go:74 +0x103
created by github.com/getlantern/flashlight/vendor/github.com/getlantern/event.NewDispatcher
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/event/event.go:39 +0x87

goroutine 18 [select, 9 minutes]:
github.com/getlantern/flashlight/vendor/github.com/getlantern/kcp-go.(*updateHeap).updateTask(0x1ac8200)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/kcp-go/updater.go:81 +0x291
created by github.com/getlantern/flashlight/vendor/github.com/getlantern/kcp-go.init.2
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/kcp-go/updater.go:13 +0x5f

goroutine 19 [chan receive, 9 minutes]:
github.com/getlantern/flashlight/geolookup.run()
	/lantern/src/github.com/getlantern/flashlight/geolookup/geolookup.go:81 +0x85
created by github.com/getlantern/flashlight/geolookup.init.0
	/lantern/src/github.com/getlantern/flashlight/geolookup/geolookup.go:77 +0x2b

goroutine 20 [sleep]:
time.Sleep(0xfc23ac00, 0x6)
	/usr/local/go/src/runtime/time.go:188 +0xd1
github.com/getlantern/flashlight/chained.init.0.func1()
	/lantern/src/github.com/getlantern/flashlight/chained/overhead.go:19 +0x70
created by github.com/getlantern/flashlight/chained.init.0
	/lantern/src/github.com/getlantern/flashlight/chained/overhead.go:17 +0x2b`,
			expectedOutput: `panic: (runtime.plainError) (0x11b43e0,0x14e8588)
/usr/local/go/src/time/sleep.go:137
/usr/local/go/src/runtime/syscall_windows.go:201
/lantern/src/github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows/dll_windows.go:174
/lantern/src/github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows/dll_windows.go:346
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/systray/systray_windows.go:742
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/systray/systray.go:75
/lantern/src/github.com/getlantern/flashlight/desktop/systray.go:60
/lantern/src/github.com/getlantern/flashlight/main/main.go:152`,
		},
		{
			panicMessage: `panic: runtime error: invalid memory address or nil pointer dereference
[signal 0xc0000005 code=0x0 addr=0x0 pc=0x7e3672]

goroutine 139 [running]:
github.com/getlantern/flashlight/balancer.(*balancedDial).onFailure(0x1685d9b0, 0x150d400, 0x138cb680, 0x1279b00, 0x14fa5e0, 0x1adfc00, 0x0)
	/lantern/src/github.com/getlantern/flashlight/balancer/balancer.go:434 +0x272
github.com/getlantern/flashlight/balancer.(*balancedDial).dialWithDialer(0x1685d9b0, 0x1502f40, 0x16878900, 0x150d400, 0x138cb680, 0x176af81c, 0xbfc5934e, 0x6d30a751, 0x1, 0x1acc6c0, ...)
	/lantern/src/github.com/getlantern/flashlight/balancer/balancer.go:370 +0x902
github.com/getlantern/flashlight/balancer.(*balancedDial).dial(0x1685d9b0, 0x1502f80, 0x1613d000, 0x176af81c, 0xbfc5934e, 0x6d30a751, 0x1, 0x1acc6c0, 0x0, 0x0, ...)
	/lantern/src/github.com/getlantern/flashlight/balancer/balancer.go:325 +0x14b
github.com/getlantern/flashlight/balancer.(*Balancer).DialContext(0x139b50e0, 0x1502f80, 0x1613d000, 0x1279ba3, 0xa, 0x13b66a00, 0x14, 0x0, 0x0, 0x0, ...)
	/lantern/src/github.com/getlantern/flashlight/balancer/balancer.go:275 +0x4bc
github.com/getlantern/flashlight/client.(*Client).doDial.func2(0x1502f80, 0x1613d000, 0x1278029, 0x8, 0x13b66a00, 0x14, 0x13b72c90, 0x0, 0x0, 0x0)
	/lantern/src/github.com/getlantern/flashlight/client/client.go:440 +0xd9
github.com/getlantern/flashlight/client.(*Client).doDial(0x138d6680, 0x13f65ff8, 0x1502f80, 0x1613d000, 0x1502f00, 0x13b66a00, 0x14, 0x0, 0x0, 0x0, ...)
	/lantern/src/github.com/getlantern/flashlight/client/client.go:487 +0xbfb
github.com/getlantern/flashlight/client.(*Client).dial(0x138d6680, 0x1503180, 0x13f65f38, 0x1192800, 0x12730bb, 0x3, 0x13b66a00, 0x14, 0x0, 0x0, ...)
	/lantern/src/github.com/getlantern/flashlight/client/client.go:411 +0x163
github.com/getlantern/flashlight/vendor/github.com/getlantern/proxy.(*proxy).requestAwareDial(0x13b172a0, 0x1503180, 0x13f65f38, 0x12730bb, 0x3, 0x13b66a00, 0x14, 0x7f43dd, 0x13de6f20, 0x1192880, ...)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/proxy/proxy_http.go:154 +0x5c
net/http.(*Transport).dial(0x1612e6c0, 0x1503180, 0x13f65f38, 0x12730bb, 0x3, 0x13b66a00, 0x14, 0x0, 0x73594000, 0x1390da80, ...)
	/usr/local/go/src/net/http/transport.go:1085 +0x151
net/http.(*Transport).dialConn(0x1612e6c0, 0x1503180, 0x13f65f38, 0x0, 0x127383c, 0x4, 0x13b66a00, 0x14, 0x0, 0x13ed2fa0, ...)
	/usr/local/go/src/net/http/transport.go:1519 +0x15a1
net/http.(*Transport).dialConnFor(0x1612e6c0, 0x161421e0)
	/usr/local/go/src/net/http/transport.go:1365 +0x72
created by net/http.(*Transport).queueForDial
	/usr/local/go/src/net/http/transport.go:1334 +0x2b8

goroutine 1 [syscall, locked to thread]:
syscall.Syscall6(0x76fb78e2, 0x4, 0x13c84f00, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, ...)
	/usr/local/go/src/runtime/syscall_windows.go:201 +0xbb
github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows.(*Proc).Call(0x13c8a180, 0x13bf3f40, 0x4, 0x4, 0x10, 0x1161d20, 0x1, 0x13bf3f40)
	/lantern/src/github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows/dll_windows.go:174 +0x28b
github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows.(*LazyProc).Call(0x13b16680, 0x13bf3f40, 0x4, 0x4, 0x13834001, 0x13c880e0, 0xb5bb44, 0x11a8080)
	/lantern/src/github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows/dll_windows.go:346 +0x48
github.com/getlantern/flashlight/vendor/github.com/getlantern/systray.nativeLoop()
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/systray/systray_windows.go:742 +0x10f
github.com/getlantern/flashlight/vendor/github.com/getlantern/systray.Run(0x13c880d8, 0x13c8a070)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/systray/systray.go:75 +0x2e
github.com/getlantern/flashlight/desktop.RunOnSystrayReady(0x11aa400, 0x150a1c0, 0x13b56060, 0x13c880d8)
	/lantern/src/github.com/getlantern/flashlight/desktop/systray.go:60 +0x64
main.main()
	/lantern/src/github.com/getlantern/flashlight/main/main.go:152 +0x299`,
			expectedOutput: `panic: runtime error: invalid memory address or nil pointer dereference
/lantern/src/github.com/getlantern/flashlight/balancer/balancer.go:434
/lantern/src/github.com/getlantern/flashlight/balancer/balancer.go:370
/lantern/src/github.com/getlantern/flashlight/balancer/balancer.go:325
/lantern/src/github.com/getlantern/flashlight/balancer/balancer.go:275
/lantern/src/github.com/getlantern/flashlight/client/client.go:440
/lantern/src/github.com/getlantern/flashlight/client/client.go:487
/lantern/src/github.com/getlantern/flashlight/client/client.go:411
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/proxy/proxy_http.go:154
/usr/local/go/src/net/http/transport.go:1085
/usr/local/go/src/net/http/transport.go:1519
/usr/local/go/src/net/http/transport.go:1365
/usr/local/go/src/net/http/transport.go:1334`,
		},
		{
			panicMessage: `panic: runtime error: slice bounds out of range [:1452] with capacity 0

goroutine 868 [running]:
github.com/getlantern/flashlight/vendor/github.com/lucas-clemente/quic-go.getPacketBuffer(...)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lucas-clemente/quic-go/buffer_pool.go:65
github.com/getlantern/flashlight/vendor/github.com/lucas-clemente/quic-go.(*packetHandlerMap).listen(0x14abe120)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lucas-clemente/quic-go/packet_handler_map.go:226 +0x147
created by github.com/getlantern/flashlight/vendor/github.com/lucas-clemente/quic-go.newPacketHandlerMap
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lucas-clemente/quic-go/packet_handler_map.go:63 +0x149

goroutine 1 [syscall, 19 minutes, locked to thread]:
syscall.Syscall6(0x768278f2, 0x4, 0x13ede0c0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, ...)
	/usr/local/go/src/runtime/syscall_windows.go:201 +0xbb
github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows.(*Proc).Call(0x13d8c1d0, 0x13d86350, 0x4, 0x4, 0x10, 0x115ec20, 0x14f7b01, 0x13d86350)
	/lantern/src/github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows/dll_windows.go:174 +0x28b
github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows.(*LazyProc).Call(0x1401e280, 0x13d86350, 0x4, 0x4, 0x0, 0x30, 0x14f7b30, 0x1adc9e0)
	/lantern/src/github.com/getlantern/flashlight/vendor/golang.org/x/sys/windows/dll_windows.go:346 +0x48
github.com/getlantern/flashlight/vendor/github.com/getlantern/systray.nativeLoop()
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/systray/systray_windows.go:742 +0x10f
github.com/getlantern/flashlight/vendor/github.com/getlantern/systray.Run(0x13d6be90, 0x13c8cf10)
	/lantern/src/github.com/getlantern/flashlight/vendor/github.com/getlantern/systray/systray.go:75 +0x2e
github.com/getlantern/flashlight/desktop.RunOnSystrayReady(0x11a7300, 0x1506b60, 0x13dac060, 0x13d6be90)
	/lantern/src/github.com/getlantern/flashlight/desktop/systray.go:60 +0x64
main.main()
	/lantern/src/github.com/getlantern/flashlight/main/main.go:152 +0x299`,
			expectedOutput: `panic: runtime error: slice bounds out of range [:1452] with capacity 0
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lucas-clemente/quic-go/buffer_pool.go:65
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lucas-clemente/quic-go/packet_handler_map.go:226
/lantern/src/github.com/getlantern/flashlight/vendor/github.com/lucas-clemente/quic-go/packet_handler_map.go:63`,
		},
	}
	for _, pTest := range panicTests {
		event := sentrySDK.Event{
			Message: pTest.panicMessage,
		}

		output := generateFingerprint(&event)
		assert.Equal(t, pTest.expectedOutput, output[1])
	}
}

type exceptionTest struct {
	exceptionValue   string
	expectedOutput string
}

func TestSentryException(t *testing.T) {
	var exceptionTests = []exceptionTest{
		{
			exceptionValue: `Unable to set up sysproxy setting tool: Unable to extract helper tool: Unable to truncate C:\Users\ANAND\AppData\Roaming\byteexec\sysproxy-cmd.exe: open C:\Users\ANAND\AppData\Roaming\byteexec\sysproxy-cmd.exe: The process cannot access the file because it is being used by another process.`,
			expectedOutput: `Unable to set up sysproxy setting tool: Unable to extract helper tool: Unable to truncate sysproxy-cmd.exe: open sysproxy-cmd.exe: The process cannot access the file because it is being used by another process.`,
		},
		{
			exceptionValue: `Unable to set up sysproxy setting tool: Unable to execute /Users/qmei/Library/Application Support/byteexec/lantern: exit status 1`,
			expectedOutput: `Unable to set up sysproxy setting tool: Unable to execute lantern: exit status 1`,
		},
		{
			exceptionValue: `Unable to accept connection: accept tcp 127.0.0.1:50292: setsockopt: A system call has failed.`,
			expectedOutput: `Unable to accept connection: accept tcp : setsockopt: A system call has failed.`,
		},
	}
	for _, eTest := range exceptionTests {
		event := sentrySDK.Event{
			Exception: []sentrySDK.Exception{{Value: eTest.exceptionValue}},
		}

		output := generateFingerprint(&event)
		assert.Equal(t, eTest.expectedOutput, output[2])
	}
}
