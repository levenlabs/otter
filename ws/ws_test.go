package ws

import (
	"net/http/httptest"
	"net/url"
	. "testing"

	"github.com/levenlabs/golib/testutil"
	"github.com/levenlabs/otter/conn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
)

func init() {
	srv := httptest.NewServer(NewHandler())
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	testRawConn = func() *websocket.Conn {
		c, err := websocket.Dial(u.String(), "", u.String())
		if err != nil {
			panic(err)
		}
		return c
	}
}

var testRawConn func() *websocket.Conn

func testConn() *websocket.Conn {
	c := testRawConn()
	var cc conn.Conn
	if err := websocket.JSON.Receive(c, &cc); err != nil {
		panic(err)
	}
	return c
}

func requireRcv(t *T, c *websocket.Conn, i interface{}) {
	err := websocket.JSON.Receive(c, i)
	require.Nil(t, err)
}

func requireWrite(t *T, c *websocket.Conn, i interface{}) {
	err := websocket.JSON.Send(c, i)
	require.Nil(t, err)
}

func TestNewConn(t *T) {
	c := testRawConn()
	var cc conn.Conn
	requireRcv(t, c, &cc)
	assert.NotEmpty(t, cc.ID)
	assert.Empty(t, cc.Presence)

	rc, ok := getRConn(cc.ID)
	assert.True(t, ok)

	c.Close()
	_, ok = <-rc.closeCh
	assert.False(t, ok)
	_, ok = getRConn(cc.ID)
	assert.False(t, ok)
}

func TestEcho(t *T) {
	c := testConn()
	out := CommandEcho{
		Command: Command{Command: "echo"},
		Message: testutil.RandStr(),
	}
	requireWrite(t, c, out)
	var in CommandEcho
	requireRcv(t, c, &in)
	assert.Equal(t, out, in)
}
