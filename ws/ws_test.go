package ws

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"net/url"
	. "testing"
	"time"

	"github.com/levenlabs/go-llog"
	"github.com/levenlabs/golib/testutil"
	"github.com/levenlabs/otter/auth"
	"github.com/levenlabs/otter/conn"
	"github.com/levenlabs/otter/distr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
)

func init() {
	llog.SetLevel(llog.DebugLevel)

	distr.Init("127.0.0.1:6379", 1, 3)
	Init(3)
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

	// We do this to ensure the tests don't get hung on something
	go func() {
		time.Sleep(5 * time.Second)
		panic("tests timedout")
	}()
}

var testRawConn func() *websocket.Conn

func testConn() (*websocket.Conn, conn.ID) {
	c := testRawConn()
	var cc conn.Conn
	if err := websocket.JSON.Receive(c, &cc); err != nil {
		panic(err)
	}
	return c, cc.ID
}

func testBackendConn() (*websocket.Conn, conn.ID) {
	c, cID := testConn()
	args := CommandAuth{
		Command:   Command{Command: "auth"},
		Signature: Auth.Sign(string(cID)),
	}
	if err := websocket.JSON.Send(c, args); err != nil {
		panic(err)
	}
	return c, cID
}

func requireRcv(t *T, c *websocket.Conn, i interface{}) {
	err := websocket.JSON.Receive(c, i)
	require.Nil(t, err)
}

func requireNoRcv(t *T, c *websocket.Conn) {
	c.SetDeadline(time.Now().Add(100 * time.Millisecond))
	err := websocket.JSON.Receive(c, nil)
	assert.True(t, err.(*net.OpError).Timeout())
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

	args = CommandAuth{
		Command:   Command{Command: "auth"},
		Signature: Auth.Sign(string(id)),
	}
	requireWrite(t, c, args)
	time.Sleep(20 * time.Millisecond)

	cc, err = distr.GetConn(id)
	require.Nil(t, err)
	assert.Equal(t, conn.Conn{ID: id, IsBackend: true}, cc)
}

func testPubCmd(ch, msg string) CommandPub {
	b, _ := json.Marshal(msg)
	msgj := json.RawMessage(b)
	return CommandPub{
		Command: Command{Command: "pub"},
		Channel: ch,
		Message: &msgj,
	}
}

func TestPubSubUnsub(t *T) {
	c1, id1 := testConn()
	c2, _ := testConn()
	cb, idb := testBackendConn()
	ch := testutil.RandStr()

	requireWrite(t, cb, CommandSub{Command: Command{Command: "sub"}, Channel: ch})
	requireWrite(t, c1, CommandSub{Command: Command{Command: "sub"}, Channel: ch})
	requireWrite(t, c2, CommandSub{Command: Command{Command: "sub"}, Channel: ch})
	// TODO make sure cb gets sub pushes for c1 and c2
	time.Sleep(100 * time.Millisecond)

	assertPubEqual := func(from conn.ID, fromBackend bool, msg string, p distr.Pub) {
		b, _ := json.Marshal(msg)
		msgj := json.RawMessage(b)
		exp := distr.Pub{
			Conn:    conn.Conn{ID: from, IsBackend: fromBackend},
			Channel: ch,
			Message: &msgj,
		}
		assert.Equal(t, exp, p)
	}

	//////////////////////////
	// Publish from client, backend should get it

	msg := testutil.RandStr()
	requireWrite(t, c1, testPubCmd(ch, msg))

	var pb distr.Pub
	requireRcv(t, cb, &pb)
	assertPubEqual(id1, false, msg, pb)

	//////////////////////////
	// Publish from backend, both clients should get it

	msg = testutil.RandStr()
	requireWrite(t, cb, testPubCmd(ch, msg))

	var p1 distr.Pub
	requireRcv(t, c1, &p1)
	assertPubEqual(idb, true, msg, p1)

	var p2 distr.Pub
	requireRcv(t, c2, &p2)
	assertPubEqual(idb, true, msg, p2)

	//////////////////////////
	// Unsub c1, then publish from backend, only c2 should get it

	requireWrite(t, c1, CommandUnsub{Command: Command{Command: "unsub"}, Channel: ch})

	// TODO make sure cb gets an unsub push for c1

	msg = testutil.RandStr()
	requireWrite(t, cb, testPubCmd(ch, msg))

	requireNoRcv(t, c1)

	p2 = distr.Pub{}
	requireRcv(t, c2, &p2)
	assertPubEqual(idb, true, msg, p2)

	//////////////////////////
	// Close c2, then publish from backend, nothing should get it

	c2.Close()

	// TODO make sure cb gets an unsub push for c2

	msg = testutil.RandStr()
	requireWrite(t, cb, testPubCmd(ch, msg))

	requireNoRcv(t, c1)
}
