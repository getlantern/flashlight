package ios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLRUConnList(t *testing.T) {
	a := noopCloser("a")
	b := noopCloser("b")
	c := noopCloser("c")

	list := newLRUConnList()
	list.mark(a)
	list.mark(b)
	list.mark(c)

	list.remove(a)
	removed, ok := list.removeOldest()
	require.True(t, ok)
	assert.Equal(t, removed, b)
	removed, ok = list.removeOldest()
	require.True(t, ok)
	assert.Equal(t, removed, c)
	_, ok = list.removeOldest()
	assert.False(t, ok)
}

type noopCloser string

func (c noopCloser) Close() error {
	return nil
}
