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
	c2, id2 := testConn()
	cb, idb := testBackendConn()
	ch := testutil.RandStr()

	requireWrite(t, cb, CommandSub{Command: Command{Command: "sub"}, Channel: ch})
	time.Sleep(100 * time.Millisecond)

	requireWrite(t, c1, CommandSub{Command: Command{Command: "sub"}, Channel: ch})
	requireWrite(t, c2, CommandSub{Command: Command{Command: "sub"}, Channel: ch})
	time.Sleep(100 * time.Millisecond)

	assertPubEqual := func(typ distr.PubType, from conn.ID, fromBackend bool, msg string, p distr.Pub) {
		exp := distr.Pub{
			Type:    typ,
			Conn:    conn.Conn{ID: from, IsBackend: fromBackend},
			Channel: ch,
		}
		if msg != "" {
			b, _ := json.Marshal(msg)
			msgj := json.RawMessage(b)
			exp.Message = &msgj
		}
		assert.Equal(t, exp, p)
	}

	//////////////////////////
	// Make sure backend receives sub messages from both clients

	var pb distr.Pub

	for i := 0; i < 2; i++ {
		pb = distr.Pub{}
		requireRcv(t, cb, &pb)
		assert.True(t, pb.Conn.ID == id1 || pb.Conn.ID == id2)
		pb.Conn.ID = ""
		assertPubEqual(distr.PubTypeSub, "", false, "", pb)
	}

	//////////////////////////
	// Publish from client, backend should get it

	msg := testutil.RandStr()
	requireWrite(t, c1, testPubCmd(ch, msg))

	pb = distr.Pub{}
	requireRcv(t, cb, &pb)
	assertPubEqual(distr.PubTypePub, id1, false, msg, pb)

	//////////////////////////
	// Publish from backend, both clients should get it

	msg = testutil.RandStr()
	requireWrite(t, cb, testPubCmd(ch, msg))

	var p1 distr.Pub
	requireRcv(t, c1, &p1)
	assertPubEqual(distr.PubTypePub, idb, true, msg, p1)

	var p2 distr.Pub
	requireRcv(t, c2, &p2)
	assertPubEqual(distr.PubTypePub, idb, true, msg, p2)

	//////////////////////////
	// Unsub c1, then publish from backend, only c2 should get it

	requireWrite(t, c1, CommandUnsub{Command: Command{Command: "unsub"}, Channel: ch})

	pb = distr.Pub{}
	requireRcv(t, cb, &pb)
	assertPubEqual(distr.PubTypeUnsub, id1, false, "", pb)

	msg = testutil.RandStr()
	requireWrite(t, cb, testPubCmd(ch, msg))

	requireNoRcv(t, c1)

	p2 = distr.Pub{}
	requireRcv(t, c2, &p2)
	assertPubEqual(distr.PubTypePub, idb, true, msg, p2)

	//////////////////////////
	// Close c2, then publish from backend, nothing should get it

	c2.Close()

	pb = distr.Pub{}
	requireRcv(t, cb, &pb)
	assertPubEqual(distr.PubTypeUnsub, id2, false, "", pb)

	msg = testutil.RandStr()
	requireWrite(t, cb, testPubCmd(ch, msg))

	requireNoRcv(t, c1)
}
