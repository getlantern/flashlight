package balancer

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEMA(t *testing.T) {
	e := newEMA(0, 0.9)
	assert.EqualValues(t, 0, e.get())
	assert.EqualValues(t, 9, e.update(10))
	assert.EqualValues(t, 9, e.get())
	assert.EqualValues(t, 18.9, e.update(20))
	assert.EqualValues(t, 18.9, e.get())
}
