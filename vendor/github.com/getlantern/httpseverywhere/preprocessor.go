package httpseverywhere

import (
	"bytes"
	"encoding/gob"
	"encoding/xml"
	"io/ioutil"
	"path/filepath"
	"regexp"

	"github.com/getlantern/golog"
)

// Preprocessor is a struct for preprocessing rules into a GOB file.
var Preprocessor = &preprocessor{
	log: golog.LoggerFor("httpseverywhere-preprocessor"),
}

type preprocessor struct {
	log golog.Logger
}

// Preprocess adds all of the rules in the specified directory.
func (p *preprocessor) Preprocess(dir string) {
	p.preprocess(dir, gobrules)
}

// preprocess adds all of the rules in the specified directory and writes to
// the specified file.
func (p *preprocessor) preprocess(dir string, outFile string) {
	rules := make([]*Ruleset, 0)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		p.log.Fatal(err)
	}

	var num int
	var errors int
	for _, file := range files {
		b, errr := ioutil.ReadFile(filepath.Join(dir, file.Name()))
		if errr != nil {
			//log.Errorf("Error reading file: %v", err)
		} else {
			rs, processed := p.VetRuleSet(b)
			if !processed {
				errors++
			} else {
				rules = append(rules, rs)
			}
		}
		num++
	}

	p.log.Debugf("Total rule set files: %v", num)
	p.log.Debugf("Loaded rules with %v rulesets and %v errors", len(rules), errors)

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	// Encode (send) the value.
	err = enc.Encode(rules)
	if err != nil {
		p.log.Fatalf("encode error: %v", err)
	}
	ioutil.WriteFile(outFile, buf.Bytes(), 0644)
}

// VetRuleSet just checks to make sure all the regular expressions compile for
// a given rule set. If any fail, we just ignore it.
func (p *preprocessor) VetRuleSet(rules []byte) (*Ruleset, bool) {
	var ruleset Ruleset
	xml.Unmarshal(rules, &ruleset)

	// If the rule is turned off, ignore it.
	if len(ruleset.Off) > 0 {
		return nil, false
	}

	// We don't run on any platforms (aka Tor) that support mixed content, so
	// ignore any rule that is mixedcontent-only.
	if ruleset.Platform == "mixedcontent" {
		return nil, false
	}

	for _, rule := range ruleset.Rule {
		_, err := regexp.Compile(rule.From)
		if err != nil {
			p.log.Debugf("Could not compile From rule %v - got error %v", rule.From, err)
			return nil, false
		}
		rule.To = p.normalizeTo(rule.To)
	}

	for _, e := range ruleset.Exclusion {
		_, err := regexp.Compile(e.Pattern)
		if err != nil {
			p.log.Debugf("Could not compile Exclusion pattern %v - got error %v", e.Pattern, err)
			return nil, false
		}
	}

	return &ruleset, true
}

func (p *preprocessor) normalizeTo(to string) string {
	// Go handles references to matching groups in the replacement text
	// differently from PCRE. PCRE considers $1xxx to be the first match
	// followed by xxx, whereas in Go that's considered to be the named group
	// "$1xxx".
	// See: https://golang.org/pkg/regexp/#Regexp.Expand
	re := regexp.MustCompile("\\$(\\d+)")
	return re.ReplaceAllString(to, "${$1}")
}
