package conn

import (
	. "testing"

	"github.com/levenlabs/golib/testutil"
	"github.com/stretchr/testify/assert"
)

func init() {
	NodeID = testutil.RandStr()
}

func TestNodeID(t *T) {
	c := New()
	nid := c.NodeID()
	assert.NotEmpty(t, nid)
	assert.Equal(t, NodeID, nid)
}
