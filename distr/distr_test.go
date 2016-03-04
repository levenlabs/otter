package distr

import (
	. "testing"
	"time"

	"github.com/levenlabs/golib/testutil"
	"github.com/levenlabs/otter/conn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	Init("127.0.0.1:6379", 1)
}

func TestGetSetConn(t *T) {
	c := conn.New()
	c.Presence = testutil.RandStr()

	require.Nil(t, SetConn(c, 100*time.Millisecond))
	c2, err := GetConn(c.ID)
	require.Nil(t, err)
	assert.Equal(t, c, c2)

	time.Sleep(500 * time.Millisecond)
	c2, err = GetConn(c.ID)
	require.Nil(t, err)
	assert.Equal(t, conn.Conn{}, c2)

	require.Nil(t, SetConn(c, 100*time.Second))
	require.Nil(t, UnsetConn(c))
	c2, err = GetConn(c.ID)
	require.Nil(t, err)
	assert.Equal(t, conn.Conn{}, c2)
}
