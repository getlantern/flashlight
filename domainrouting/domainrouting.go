package domainrouting

import (
	"sync"
	"time"

	"github.com/getlantern/detour"
	"github.com/getlantern/domains"
	"github.com/getlantern/eventual"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight.domainrouting")

	currentRules = eventual.NewValue()
	mx           sync.RWMutex
)

const (
	// how long to wait for initialization of rules when checking rules
	initTimeout = 30 * time.Second
)

var (
	// Rules
	None   = Rule("")
	Direct = Rule("d")
	Proxy  = Rule("p")
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

// Configure configures domain routing with the new Rules and ProxiedSitesConfig. The ProxiedSitesConfig is supported
// for backwards compatibility. All domains in the ProxiedSitesConfig are treated as Proxy rules.
func Configure(newRules Rules, proxiedSites *ProxiedSitesConfig) {
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

	updated := update(getCurrentRules(0), newRules, func(domain string, oldRule, newRule Rule) {
		// maintain detour
		if oldRule == Proxy {
			log.Tracef("Removing from detour whitelist: %v", domain)
			detour.RemoveFromWl(domain)
		} else if newRule == Proxy {
			log.Tracef("Adding to detour whitelist: %v", domain)
			detour.AddToWl(domain, true)
		}
	})
	currentRules.Set(updated)

	mx.Unlock()
}

// ShouldSendDirect identifies whether traffic to the given domain should always be sent direct (i.e. bypass proxy)
func ShouldSendDirect(domain string) bool {
	mx.RLock()
	rules := getCurrentRules(initTimeout)
	mx.RUnlock()

	if rules == nil {
		log.Debugf("domainrouting not initialized within %v, assuming not to send direct", initTimeout)
		return false
	}

	_rule, found := rules.BestMatch(domain)
	return found && _rule == Direct
}

func getCurrentRules(timeout time.Duration) *domains.Tree {
	current, found := currentRules.Get(timeout)
	if !found {
		return nil
	}
	return current.(*domains.Tree)
}

func update(oldRules *domains.Tree, newRules Rules, onChange func(domain string, oldRule, newRule Rule)) *domains.Tree {
	loggingOnChange := func(domain string, oldRule, newRule Rule) {
		if newRule == Direct {
			log.Debugf("Will force direct traffic for %v", domain)
		} else if oldRule == Direct {
			log.Debugf("Will no longer force direct traffic for %v", domain)
		}
		onChange(domain, oldRule, newRule)
	}

	result := domains.NewTree()

	if oldRules == nil {
		// Everything is new, report changes accordingly
		for domain, newRule := range newRules {
			result.Insert(dotted(domain), newRule)
			loggingOnChange(domain, None, Rule(newRule))
		}
		return result
	}

	for domain, newRule := range newRules {
		dd := dotted(domain)
		result.Insert(dd, newRule)
		_oldRule, found := oldRules.Get(dd)
		var oldRule Rule
		if found {
			oldRule = _oldRule.(Rule)
		}
		if newRule != oldRule {
			loggingOnChange(domain, oldRule, Rule(newRule))
		}
	}

	oldRules.Walk(func(dd string, oldRule interface{}) bool {
		domain := undotted(dd)
		newRule, hasNewRule := newRules[domain]
		if !hasNewRule {
			loggingOnChange(domain, oldRule.(Rule), Rule(newRule))
		}
		return true
	})

	return result
}

// dotted prefixes domains with a dot to enable prefix matching (which we do for all of our rules)
func dotted(domain string) string {
	return "." + domain
}

func undotted(domain string) string {
	return domain[1:]
}
