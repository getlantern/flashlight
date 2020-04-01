package domainrouting

import (
	"sync"

	"github.com/getlantern/detour"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight.domainrouting")

	currentRules Rules
	mx           sync.RWMutex
)

const (
	// Rules
	None   = ""
	Direct = "d"
	Proxy  = "p"
)

// ProxiedSitesConfig is a legacy config structure that provides backwards compatibility with old config formats
type ProxiedSitesConfig struct {
	// Global list of white-listed sites
	Cloud []string
}

// Rule specifies how we should handle traffic to a domain and its sub-domains
type Rule string

// Rules maps domains to their corresponding rules
type Rules map[string]Rule

// Update updates a given Rules from a set of newRules, calling the provided onChange function for any applied changes.
func (oldRules Rules) Update(newRules Rules, onChange func(domain string, oldRule, newRule Rule)) Rules {
	if oldRules == nil {
		oldRules = make(Rules)
	}

	for domain, newRule := range newRules {
		oldRule, _ := oldRules[domain]
		if newRule != oldRule {
			onChange(domain, Rule(oldRule), Rule(newRule))
		}
	}

	for domain, oldRule := range oldRules {
		newRule, hasNewRule := newRules[domain]
		if !hasNewRule {
			onChange(domain, Rule(oldRule), Rule(newRule))
		}
	}

	return newRules
}

func Configure(newRules Rules, proxiedSites *ProxiedSitesConfig) {
	log.Debug("Configuring")

	// For backwards compatibility, merge in ProxiedSites
	if newRules == nil {
		newRules = make(Rules)
	}

	for _, domain := range proxiedSites.Cloud {
		_, alreadyDefined := newRules[domain]
		if !alreadyDefined {
			newRules[domain] = Proxy
		}
	}

	mx.Lock()
	// TODO: subscribe changes of geolookup and set country accordingly
	// safe to hardcode here as IR has all detection rules
	detour.SetCountry("IR")

	currentRules = currentRules.Update(newRules, func(domain string, oldRule, newRule Rule) {
		// maintain detour
		if oldRule == Proxy {
			log.Tracef("Removing from detour whitelist: %v", domain)
			detour.RemoveFromWl(domain)
		} else if newRule == Proxy {
			log.Tracef("Adding to detour whitelist: %v", domain)
			detour.AddToWl(domain, true)
		}
	})
	mx.Unlock()
}
