package shortcut

import (
	"bytes"
	"testing"

	"github.com/getlantern/golog"
	"github.com/getlantern/shortcut"
	"github.com/stretchr/testify/assert"
)

func TestReadResources(t *testing.T) {
	log := golog.LoggerFor("shortcut-test")
	countries := []string{"ae", "cn", "ir", "default"}
	for _, country := range countries {
		v4, v4err := ipTables.ReadFile("resources/" + country + "_ipv4.txt")
		assert.Nil(t, v4err)
		v6, v6err := ipTables.ReadFile("resources/" + country + "_ipv6.txt")
		assert.Nil(t, v6err)
		sc := shortcut.NewFromReader(
			bytes.NewReader(v4),
			bytes.NewReader(v6),
		)
		log.Debugf("Initialized shortcut for '%s':\n\t%v", country, sc)

	}
}
