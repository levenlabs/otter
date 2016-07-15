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

func TestSubUnsub(t *T) {
	c := conn.New()
	cb := conn.New()
	cb.IsBackend = true
	ch := testutil.RandStr()

	assertSubscribed := func(timeout time.Duration, clients, backend []conn.Conn) {
		l, err := GetSubscribed(conn.NodeID, ch, false, timeout)
		assert.Nil(t, err)
		assert.Equal(t, clients, l)

		l, err = GetSubscribed(conn.NodeID, ch, true, timeout)
		assert.Nil(t, err)
		assert.Equal(t, backend, l)
	}

	assertSubscribed(1*time.Second, []conn.Conn{}, []conn.Conn{})

	require.Nil(t, Subscribe(c, ch))
	assertSubscribed(1*time.Second, []conn.Conn{c}, []conn.Conn{})
	require.Nil(t, Subscribe(cb, ch))
	assertSubscribed(1*time.Second, []conn.Conn{c}, []conn.Conn{cb})

	// Make sure duplicate subscribing doesn't do anything
	require.Nil(t, Subscribe(c, ch))
	assertSubscribed(1*time.Second, []conn.Conn{c}, []conn.Conn{cb})
	require.Nil(t, Subscribe(cb, ch))
	assertSubscribed(1*time.Second, []conn.Conn{c}, []conn.Conn{cb})

	// Make sure timeout on GetSubscribed does the right thing
	time.Sleep(100 * time.Millisecond)
	assertSubscribed(100*time.Millisecond, []conn.Conn{}, []conn.Conn{})

	require.Nil(t, Unsubscribe(c, ch))
	assertSubscribed(1*time.Second, []conn.Conn{}, []conn.Conn{cb})
	require.Nil(t, Unsubscribe(cb, ch))
	assertSubscribed(1*time.Second, []conn.Conn{}, []conn.Conn{})

	// Make sure cleanup works correctly
	require.Nil(t, Subscribe(c, ch))
	require.Nil(t, Subscribe(cb, ch))
	time.Sleep(100 * time.Millisecond)
	CleanChannels(true, 100*time.Millisecond)
	CleanChannels(false, 100*time.Millisecond)
	zcount, err := cmder.Cmd("ZCARD", channelKey(conn.NodeID, ch, false)).Int()
	require.Nil(t, err)
	assert.Zero(t, zcount)
	zcount, err = cmder.Cmd("ZCARD", channelKey(conn.NodeID, ch, true)).Int()
	require.Nil(t, err)
	assert.Zero(t, zcount)
}
