package httpseverywhere

import "regexp"

// Target is the target host for a given rule.
type Target struct {
	Host string `xml:"host,attr"`
}

// Exclusion is a RE pattern to ignore when processing a rule set.
type Exclusion struct {
	Pattern string `xml:"pattern,attr"`
}

// Rule is a rule to apply when processing a URL.
type Rule struct {
	From string `xml:"from,attr"`
	To   string `xml:"to,attr"`
}

// Ruleset is a set of rules to apply to a set of targets with flags for things
// like whether or not the set is active, targets, rules, exclusions, etc.
type Ruleset struct {
	Off       string       `xml:"default_off,attr"`
	Platform  string       `xml:"platform,attr"`
	Target    []*Target    `xml:"target"`
	Exclusion []*Exclusion `xml:"exclusion"`
	Rule      []*Rule      `xml:"rule"`
}

// The below types are simplified in-memory representations for what we
// actually need at runtime after rules have been deserialized.

// exclusion is a RE pattern to ignore when processing a rule set.
type exclusion struct {
	pattern *regexp.Regexp
}

// rule is a rule to apply when processing a URL.
type rule struct {
	from *regexp.Regexp
	to   string
}

// ruleset is a set of rules to apply to a set of targets with flags for things
// like whether or not the set is active, targets, rules, exclusions, etc.
type ruleset struct {
	exclusion []exclusion
	rule      []rule
}
