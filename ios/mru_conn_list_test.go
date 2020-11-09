package ios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMRUConnList(t *testing.T) {
	a := noopCloser("a")
	b := noopCloser("b")
	c := noopCloser("c")

	list := newMRUConnList()
	list.mark(a)
	list.mark(b)
	list.mark(c)

	list.remove(c)
	removed, ok := list.removeNewest()
	require.True(t, ok)
	assert.Equal(t, removed, b)
	removed, ok = list.removeNewest()
	require.True(t, ok)
	assert.Equal(t, removed, a)
	_, ok = list.removeNewest()
	assert.False(t, ok)
}

type noopCloser string

func (c noopCloser) Close() error {
	return nil
}
