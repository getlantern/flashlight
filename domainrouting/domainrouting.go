package domainrouting

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/domains"
	"github.com/getlantern/eventual/v2"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight.domainrouting")

	// Invariant: ruleSet is always the value used to create ruleTree (via buildTree).
	ruleSet  = eventual.NewValue() // RuleSet
	ruleTree = eventual.NewValue() // *domains.Tree
	mx       sync.RWMutex

	// Closed when Configure is called for the first time.
	configured          = make(chan struct{})
	closeConfiguredOnce sync.Once
)

const (
	// how long to wait for initialization of rules when checking rules
	initTimeout = 5 * time.Second
)

// Requests to these domains require proxies for security purposes and to
// ensure that config-server requests in particular always go through a proxy
// so that it can add the necessary authentication token and other headers.
// Direct connections to these domains are not allowed.
var domainsRequiringProxy = []string{"getiantem.org", "lantern.io", "getlantern.org"}

// ProxiedSitesConfig is a legacy config structure that provides backwards compatibility with old
// config formats
type ProxiedSitesConfig struct {
	// Global list of white-listed sites
	Cloud []string
}

// Rule specifies how traffic should be handled for a domain and its sub-domains.
type Rule interface {
	isRule()
}

// RuleDirect indicates that traffic should be sent directly to the domain.
type RuleDirect struct{}

func (rd RuleDirect) isRule() {}

// RuleProxy indicates that traffic for this domain should be proxied.
type RuleProxy struct{}

func (rp RuleProxy) isRule() {}

// RuleNone indicates that there is no configured rule for the domain.
type RuleNone struct{}

func (rn RuleNone) isRule() {}

// RuleReroute indicates that traffic for a domain should be re-routed to another domain.
type RuleReroute string

func (rr RuleReroute) isRule() {}

// RouteTo returns the domain traffic should be routed to. This function is provided for convenience
// and readability; it is equivalent to string(rr).
func (rr RuleReroute) RouteTo() string { return string(rr) }

// Rule specifies how we should handle traffic to a domain and its sub-domains
// type Rule string

// RuleSet maps domains to their corresponding rules.
//
// This type supports serialization to and from a YAML map of domains mapped as follows:
//	- RuleDirect  -> the character 'd'
//	- RuleProxy   -> the character 'p'
//	- RuleNone    -> not included
//	- RuleReroute -> the domain to route to
//
// See the example for additional clarity.
type RuleSet map[string]Rule

// GetYAML implements yaml.Getter.
func (r RuleSet) GetYAML() (tag string, value interface{}) {
	m := map[string]string{}
	for key, rule := range r {
		switch v := rule.(type) {
		case RuleDirect:
			m[key] = "d"
		case RuleProxy:
			m[key] = "p"
		case RuleNone:
			// Don't include.
		case RuleReroute:
			m[key] = v.RouteTo()
		}
	}
	return "", m
}

// SetYAML implements yaml.Setter.
func (r RuleSet) SetYAML(_ string, value interface{}) bool {
	m, ok := value.(map[interface{}]interface{})
	if !ok {
		log.Errorf("failed to unmarshal Rules: unexpected type %T", value)
		return false
	}
	r = map[string]Rule{}
	for k, v := range m {
		domain, ok := k.(string)
		if !ok {
			log.Errorf("failed to unmarshal Rules domain: unexpected type %T", k)
			continue
		}
		vStr, ok := v.(string)
		if !ok {
			log.Errorf("failed to unmarshal Rules value for '%s': unexpected type %T", domain, v)
			continue
		}
		switch vStr {
		case "d":
			r[domain] = RuleDirect{}
		case "p":
			r[domain] = RuleProxy{}
		case "":
			// Skip (equivalent to RuleNone, but shouldn't be included).
		default:
			r[domain] = RuleReroute(vStr)
		}
	}
	return true
}

// Configure configures domain routing with the new Rules and ProxiedSitesConfig. Overwrites any
// existing routing rules.
//
// The ProxiedSitesConfig is supported for backwards compatibility. All domains in the
// ProxiedSitesConfig are treated as Proxy rules.
func Configure(rules RuleSet, proxiedSites *ProxiedSitesConfig) {
	log.Debug("[3349] Configure called")
	log.Debugf("Configuring with %d rules and %d proxied sites", len(rules), len(proxiedSites.Cloud))

	// For backwards compatibility, merge in ProxiedSites
	if rules == nil {
		rules = make(RuleSet)
	}

	for _, domain := range proxiedSites.Cloud {
		_, alreadyDefined := rules[domain]
		if !alreadyDefined {
			rules[domain] = RuleProxy{}
		}
	}

	// There are certain domains that always require proxying no matter what, merge those in
	for _, domain := range domainsRequiringProxy {
		rules[domain] = RuleProxy{}
	}

	newRuleTree := buildTree(rules)
	mx.Lock()
	ruleSet.Set(rules)
	ruleTree.Set(newRuleTree)
	mx.Unlock()
	closeConfiguredOnce.Do(func() { close(configured) })
}

// WaitForConfigure waits until Configure is called for the first time. Returns an error if the
// context expires first. Use this function to determine when it is safe to call AddRule.
func WaitForConfigure(ctx context.Context) error {
	select {
	case <-configured:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// AddRule for a domain. Replaces existing rules for the domain. Use RuleNone to clear rules.
//
// Rules cannot be added via this function before Configure is called.
func AddRule(domain string, r Rule) (previous Rule, err error) {
	mx.Lock()
	defer mx.Unlock()
	_rules, err := ruleSet.Get(eventual.DontWait)
	if err != nil {
		return nil, errors.New("individual rules cannot be added before Configure is called")
	}
	rules := _rules.(RuleSet)

	previous, ok := rules[domain]
	if !ok {
		previous = RuleNone{}
	}
	if _, ok := r.(RuleNone); ok {
		delete(rules, domain)
	} else {
		rules[domain] = r
	}

	// Note that rules should already contain any defaults Configure would normally add.
	ruleSet.Set(rules)
	ruleTree.Set(buildTree(rules))

	return previous, nil
}

// RuleFor returns the Rule most applicable to the given domain. If no such rule is defined,
func RuleFor(domain string) Rule {
	log.Debugf("[3349] RuleFor(%s)", domain)

	mx.RLock()
	ctx, cancel := context.WithTimeout(context.Background(), initTimeout)
	rules := getCurrentRules(ctx)
	mx.RUnlock()
	cancel()

	return ruleFor(domain, rules)
}

func ruleFor(domain string, rules *domains.Tree) Rule {
	if rules == nil {
		log.Debugf("domainrouting not initialized within %v, assuming that domain should be proxied", initTimeout)
		return RuleProxy{}
	}

	_rule, found := rules.BestMatch(strings.ToLower(domain))
	if !found {
		return RuleNone{}
	}
	return _rule.(Rule)
}

func getCurrentRules(ctx context.Context) *domains.Tree {
	current, err := ruleTree.Get(ctx)
	if err != nil {
		return nil
	}
	return current.(*domains.Tree)
}

func buildTree(rules RuleSet) *domains.Tree {
	result := domains.NewTree()
	for domain, rule := range rules {
		if _, ok := rule.(RuleDirect); ok {
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
