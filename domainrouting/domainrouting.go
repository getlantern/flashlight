package domainrouting

import (
	"strings"
	"sync"
	"time"

	"github.com/getlantern/errors"

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
	None       = Rule("")
	Direct     = Rule("d")
	Proxy      = Rule("p")
	MustProxy  = Rule("m")
	MustDirect = Rule("md")

	// Requests to these domains require proxies for security purposes and to
	// ensure that config-server requests in particular always go through a proxy
	// so that it can add the necessary authentication token and other headers.
	// Direct connections to these domains are not allowed.
	domainsRequiringProxy = []string{"getiantem.org", "lantern.io", "getlantern.org", "ss7hc6jm.io", "beam.place", "beam.dance"}
)

// ProxiedSitesConfig is a legacy config structure that provides backwards compatibility with old config formats
type ProxiedSitesConfig struct {
	// Global list of white-listed sites
	Cloud []string
}

// Rule specifies how we should handle traffic to a domain and its sub-domains
type Rule string

// RulesMap maps domains to their corresponding rules
type RulesMap map[string]Rule

// Rules is a tree of domain-routing rules that supports fast evaluation
type Rules struct {
	tree *domains.Tree
}

// Configure configures domain routing with the new Rules and ProxiedSitesConfig. The ProxiedSitesConfig is supported
// for backwards compatibility. All domains in the ProxiedSitesConfig are treated as Proxy rules.
func Configure(rules RulesMap, proxiedSites *ProxiedSitesConfig) {
	log.Debugf("Configuring with %d rules and %d proxied sites", len(rules), len(proxiedSites.Cloud))

	// For backwards compatibility, merge in ProxiedSites
	if rules == nil {
		rules = make(RulesMap)
	}

	for _, domain := range proxiedSites.Cloud {
		_, alreadyDefined := rules[domain]
		if !alreadyDefined {
			rules[domain] = Proxy
		}
	}

	// There are certain domains that always require proxying no matter what, merge those in
	for _, domain := range domainsRequiringProxy {
		rules[domain] = MustProxy
	}

	newRules := NewRules(rules)
	mx.Lock()
	currentRules.Set(newRules)
	mx.Unlock()
}

// AddRules adds the given rules to the current rules.
func AddRules(newRules RulesMap) error {
	mx.Lock()
	defer mx.Unlock()
	rules := getCurrentRules(initTimeout)
	if rules == nil {
		return errors.New("Rules not yet initialized")
	}
	for k, v := range rules.tree.ToMap() {
		newRules[k] = v.(Rule)
	}

	currentRules.Set(NewRules(newRules))
	return nil
}

// RemoveRules removes the domains from the current rules.
func RemoveRules(oldRules RulesMap) error {
	mx.Lock()
	defer mx.Unlock()
	rules := getCurrentRules(initTimeout)
	if rules == nil {
		return errors.New("Rules not yet initialized")
	}
	m := rules.tree.ToMap()
	for domain := range oldRules {
		delete(m, dotted(strings.ToLower(domain)))
	}
	newRules := RulesMap{}
	for k, v := range m {
		newRules[k] = v.(Rule)
	}
	currentRules.Set(NewRules(newRules))
	return nil
}

// RuleFor returns the Rule most applicable to the given domain. If no such rule is defined, returns None
func RuleFor(domain string) Rule {
	mx.RLock()
	rules := getCurrentRules(initTimeout)
	mx.RUnlock()

	if rules == nil {
		log.Debugf("domainrouting not initialized within %v, assuming that domain should be proxied", initTimeout)
		return Proxy
	}

	return rules.RuleFor(domain)
}

func (rules *Rules) RuleFor(domain string) Rule {
	_rule, found := rules.tree.BestMatch(strings.ToLower(domain))
	if !found {
		return None
	}
	return _rule.(Rule)
}

func getCurrentRules(timeout time.Duration) *Rules {
	current, found := currentRules.Get(timeout)
	if !found {
		return nil
	}
	return current.(*Rules)
}

// Construct a new Rules tree
func NewRules(rules RulesMap) *Rules {
	result := domains.NewTree()
	for domain, rule := range rules {
		if rule == Direct {
			log.Debugf("Will force direct traffic for %v", domain)
		}
		if rule == MustDirect {
			log.Debugf("Will always force direct traffic for %v", domain)
		}
		result.Insert(dotted(strings.ToLower(domain)), rule)
	}
	return &Rules{result}
}

// dotted prefixes domains with a dot to enable prefix matching (which we do for all of our rules)
func dotted(domain string) string {
	return "." + domain
}
