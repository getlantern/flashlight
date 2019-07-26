package pcap

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"

	"github.com/getlantern/appdir"
	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/chained"
)

var (
	log               = golog.LoggerFor("flashlight.pcap")
	logdir            = appdir.Logs("Lantern")
	params_template   = "-ni en0 -p -w %s/%s.pcapng -C 1 -W 10 -s 200 host %s and not port ssh"
	activeProcesses   []*exec.Cmd
	muActiveProcesses sync.Mutex
)

func Configure(proxies map[string]*chained.ChainedServerInfo) {
	var processes []*exec.Cmd
	for name, config := range proxies {
		host, _, err := net.SplitHostPort(config.Addr)
		if err != nil {
			host = config.Addr
		}
		params := fmt.Sprintf(params_template, logdir, name, host)
		cmd := exec.Command("tcpdump", strings.Split(params, " ")...)
		log.Debugf("Starting '%v'", cmd.Args)
		if err := cmd.Start(); err != nil {
			log.Errorf("failed to start '%v': %v", cmd.Args, err)
		} else {
			processes = append(processes, cmd)
		}
	}
	muActiveProcesses.Lock()
	existing := activeProcesses
	activeProcesses = processes
	muActiveProcesses.Unlock()
	for _, cmd := range existing {
		if err := cmd.Process.Kill(); err != nil {
			log.Errorf("failed to kill '%v': %v", cmd.Args, err)
		}
	}
}
