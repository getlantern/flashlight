// +build darwin dragonfly freebsd !android,linux netbsd openbsd solaris

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/log"
	"github.com/getlantern/flashlight/server"
	"github.com/getlantern/flashlight/statserver"
	"github.com/getlantern/go-igdman/igdman"
)

func mapPort(cfg *config.Config) error {
	parts := strings.Split(cfg.Addr, ":")

	internalPort, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("Unable to parse local port: ")
	}

	internalIP := parts[0]
	if internalIP == "" {
		internalIP, err = determineInternalIP()
		if err != nil {
			return fmt.Errorf("Unable to determine internal IP: %s", err)
		}
	}

	igd, err := igdman.NewIGD()
	if err != nil {
		return fmt.Errorf("Unable to get IGD: %s", err)
	}

	igd.RemovePortMapping(igdman.TCP, cfg.Portmap)
	err = igd.AddPortMapping(igdman.TCP, internalIP, internalPort, cfg.Portmap, 0)
	if err != nil {
		return fmt.Errorf("Unable to map port with igdman %d: %s", cfg.Portmap, err)
	}

	return nil
}

// runServerProxy runs the server-side proxy
func runServerProxy(cfg *config.Config) {
	useAllCores()

	if cfg.Portmap > 0 {
		log.Debugf("Attempting to map external port %d", cfg.Portmap)
		err := mapPort(cfg)
		if err != nil {
			log.Errorf("Unable to map external port: %s", err)
			os.Exit(PortmapFailure)
		}
		log.Debugf("Mapped external port %d", cfg.Portmap)
	}

	srv := &server.Server{
		Addr:         cfg.Addr,
		ReadTimeout:  0, // don't timeout
		WriteTimeout: 0,
		Host:         cfg.AdvertisedHost,
		CertContext: &server.CertContext{
			PKFile:         config.InConfigDir("proxypk.pem"),
			ServerCertFile: config.InConfigDir("servercert.pem"),
		},
	}
	if cfg.StatsAddr != "" {
		// Serve stats
		srv.StatServer = &statserver.Server{
			Addr: cfg.StatsAddr,
		}
	}
	err := srv.ListenAndServe()
	if err != nil {
		log.Fatalf("Unable to run server proxy: %s", err)
	}
}
