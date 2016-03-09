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
	Init("127.0.0.1:6379", 1, 3)
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

func TestSubUnsub(t *T) {
	c := conn.New()
	cb := conn.New()
	cb.IsBackend = true
	ch := testutil.RandStr()

	assertSubscribed := func(clients, backend []conn.ID) {
		l, err := GetSubscribed(conn.NodeID, ch, false)
		assert.Nil(t, err)
		assert.Equal(t, clients, l)

		l, err = GetSubscribed(conn.NodeID, ch, true)
		assert.Nil(t, err)
		assert.Equal(t, backend, l)
	}

	assertSubscribed([]conn.ID{}, []conn.ID{})

	require.Nil(t, Subscribe(c, ch))
	assertSubscribed([]conn.ID{c.ID}, []conn.ID{})
	require.Nil(t, Subscribe(cb, ch))
	assertSubscribed([]conn.ID{c.ID}, []conn.ID{cb.ID})

	// Make sure duplicate subscribing doesn't do anythign
	require.Nil(t, Subscribe(c, ch))
	assertSubscribed([]conn.ID{c.ID}, []conn.ID{cb.ID})
	require.Nil(t, Subscribe(cb, ch))
	assertSubscribed([]conn.ID{c.ID}, []conn.ID{cb.ID})

	require.Nil(t, Unsubscribe(c, ch))
	assertSubscribed([]conn.ID{}, []conn.ID{cb.ID})
	require.Nil(t, Unsubscribe(cb, ch))
	assertSubscribed([]conn.ID{}, []conn.ID{})
}
