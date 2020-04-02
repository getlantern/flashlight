package domainrouting

import (
	"strings"
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
	initTimeout = 5 * time.Second
)

var (
	// Rules
	None   = Rule("")
	Direct = Rule("d")
	Proxy  = Rule("p")

	// Requests to these domains require proxies for security purposes and to
	// ensure that config-server requests in particular always go through a proxy
	// so that it can add the necessary authentication token and other headers.
	// Direct connections to these domains are not allowed.
	domainsRequiringProxy = []string{"getiantem.org", "lantern.io", "getlantern.org"}
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
func Configure(rules Rules, proxiedSites *ProxiedSitesConfig) {
	log.Debugf("Configuring with %d rules and %d proxied sites", len(rules), len(proxiedSites.Cloud))

	// For backwards compatibility, merge in ProxiedSites
	if rules == nil {
		rules = make(Rules)
	}

	for _, domain := range proxiedSites.Cloud {
		_, alreadyDefined := rules[domain]
		if !alreadyDefined {
			rules[domain] = Proxy
		}
	}

	// There are certain domains that always require proxying no matter what, merge those in
	for _, domain := range domainsRequiringProxy {
		rules[domain] = Proxy
	}

	newRules := buildTree(rules)
	mx.Lock()
	// TODO: subscribe changes of geolookup and set country accordingly
	// safe to hardcode here as IR has all detection rules
	detour.SetCountry("IR")
	currentRules.Set(newRules)
	mx.Unlock()
}

// RuleFor returns the Rule most applicable to the given domain. If no such rule is defined,
func RuleFor(domain string) Rule {
	mx.RLock()
	rules := getCurrentRules(initTimeout)
	mx.RUnlock()

	return ruleFor(domain, rules)
}

func ruleFor(domain string, rules *domains.Tree) Rule {
	if rules == nil {
		log.Debugf("domainrouting not initialized within %v, assuming that domain should be proxied", initTimeout)
		return Proxy
	}

	_rule, found := rules.BestMatch(strings.ToLower(domain))
	if !found {
		return None
	}
	return _rule.(Rule)
}

func getCurrentRules(timeout time.Duration) *domains.Tree {
	current, found := currentRules.Get(timeout)
	if !found {
		return nil
	}
	return current.(*domains.Tree)
}

func buildTree(rules Rules) *domains.Tree {
	result := domains.NewTree()
	for domain, rule := range rules {
		if rule == Direct {
			log.Debugf("Will force direct traffic for %v", domain)
		}
		result.Insert(dotted(strings.ToLower(domain)), rule)
	}
	return result
}

// dotted prefixes domains with a dot to enable prefix matching (which we do for all of our rules)
func dotted(domain string) string {
	return "." + domain
}
