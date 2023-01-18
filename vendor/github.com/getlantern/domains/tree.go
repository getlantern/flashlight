// Package domains provides utilities that help when working with domains
package domains

import (
	"github.com/armon/go-radix"
)

// Map is a map that maps domain names to some other data
type Map map[string]interface{}

// Tree is a radix-tree that supports domain-based matching including wildcard prefixes (specified with a leading .)
type Tree struct {
	t *radix.Tree
}

// NewTree constructs a new empty Tree
func NewTree() *Tree {
	return &Tree{radix.New()}
}

// NewTreeFromMap constructs a Tree initialized from a given Map
func NewTreeFromMap(initial Map) *Tree {
	t := radix.New()
	for domain, value := range initial {
		t.Insert(reversed(domain), value)
	}
	return &Tree{t}
}

// BestMatch finds the best match for the given domain
func (tree *Tree) BestMatch(domain string) (result interface{}, found bool) {
	rd := reversed(domain)
	// check exact match first
	result, found = tree.Get(domain)
	if found {
		return result, found
	}

	// then check wildcard match
	matchedOn, result, found := tree.t.LongestPrefix(rd + ".")
	if found && matchedOn[len(matchedOn)-1] != '.' {
		result = nil
		found = false
	}
	return result, found
}

// Insert inserts a new value into the tree
func (tree *Tree) Insert(domain string, value interface{}) {
	tree.t.Insert(reversed(domain), value)
}

// Get gets the entry exactly matching the given domain
func (tree *Tree) Get(domain string) (result interface{}, found bool) {
	return tree.t.Get(reversed(domain))
}

// Walk walks the tree, calling the supplied fn for each domain/value pair in the tree
// and continuing the walk as long as fn returns true.
func (tree *Tree) Walk(fn func(domain string, value interface{}) bool) {
	tree.t.Walk(func(key string, value interface{}) bool {
		return !fn(reversed(key), value)
	})
}

// ToMap obtains the Tree as a Map
func (tree *Tree) ToMap() Map {
	m := tree.t.ToMap()
	result := make(Map, len(m))
	for domain, value := range m {
		result[reversed(domain)] = value
	}
	return result
}

func reversed(input string) string {
	runes := []rune(input)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
