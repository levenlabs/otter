package ws

import (
	"net/http/httptest"
	"net/url"
	. "testing"
	"time"

	"github.com/levenlabs/golib/testutil"
	"github.com/levenlabs/otter/auth"
	"github.com/levenlabs/otter/conn"
	"github.com/levenlabs/otter/distr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
)

func init() {
	distr.Init("127.0.0.1:6379", 1)
	Auth = auth.Auth{Key: testutil.RandStr()}

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

func testConn() (*websocket.Conn, string) {
	c := testRawConn()
	var cc conn.Conn
	if err := websocket.JSON.Receive(c, &cc); err != nil {
		panic(err)
	}
	return c, cc.ID
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

	cc2, err := distr.GetConn(cc.ID)
	require.Nil(t, err)
	assert.Equal(t, cc, cc2)

	c.Close()
	_, ok = <-rc.closeCh
	assert.False(t, ok)
	_, ok = getRConn(cc.ID)
	assert.False(t, ok)

	cc2, err = distr.GetConn(cc.ID)
	require.Nil(t, err)
	assert.Equal(t, conn.Conn{}, cc2)
}

func TestEcho(t *T) {
	c, _ := testConn()
	out := CommandEcho{
		Command: Command{Command: "echo"},
		Message: testutil.RandStr(),
	}
	requireWrite(t, c, out)
	var in CommandEcho
	requireRcv(t, c, &in)
	assert.Equal(t, out, in)
}

func TestAuth(t *T) {
	c, id := testConn()
	pres := testutil.RandStr()
	args := CommandAuth{
		Command:   Command{Command: "auth"},
		Presence:  pres,
		Signature: Auth.Sign(pres),
	}
	requireWrite(t, c, args)
	time.Sleep(20 * time.Millisecond)

	cc, err := distr.GetConn(id)
	require.Nil(t, err)
	assert.Equal(t, conn.Conn{ID: id, Presence: pres}, cc)
}
