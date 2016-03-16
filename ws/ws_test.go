package ws

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	. "testing"
	"time"

	"github.com/levenlabs/go-llog"
	"github.com/levenlabs/golib/testutil"
	"github.com/levenlabs/otter/conn"
	"github.com/levenlabs/otter/distr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
)

func init() {
	llog.SetLevel(llog.DebugLevel)

	distr.Init("127.0.0.1:6379", 1, 3)
	Init(testutil.RandStr(), 3)

	srv := httptest.NewServer(NewHandler())
	testURL, _ = url.Parse(srv.URL)

	// We do this to ensure the tests don't get hung on something
	go func() {
		time.Sleep(5 * time.Second)
		panic("tests timedout")
	}()
}

var testURL *url.URL

func makeTestURL(scheme, presence string, subs ...string) string {
	sig := Auth.Sign(presence)
	u := *testURL
	u.Scheme = scheme
	return u.String() + "/" + strings.Join(subs, ",") + "?presence=" + presence + "&sig=" + sig
}

func testConn(backend bool, subs ...string) (*websocket.Conn, string) {
	presence := "backend"
	if !backend {
		presence = testutil.RandStr()
	}
	u := makeTestURL("ws", presence, subs...)

	c, err := websocket.Dial(u, "", u)
	if err != nil {
		panic(err)
	}
	return c, presence
}

func testPub(presence, msg string, subs ...string) {
	u := makeTestURL("http", presence, subs...)
	b, _ := json.Marshal(msg)
	r, err := http.NewRequest("POST", u, bytes.NewBuffer(b))
	if err != nil {
		panic(err)
	}
	if _, err := http.DefaultClient.Do(r); err != nil {
		panic(err)
	}
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

func TestNewConn(t *T) {
	c, presence := testConn(false)
	time.Sleep(100 * time.Millisecond)

	assert.Len(t, r, 1)
	var id conn.ID
	var rc rConn
	for id, rc = range r {
		break
	}
	require.NotEmpty(t, id)

	cc, err := distr.GetConn(id)
	require.Nil(t, err)
	assert.Equal(t, presence, cc.Presence)

	c.Close()
	_, ok := <-rc.closeCh
	assert.False(t, ok)
	_, ok = getRConn(id)
	assert.False(t, ok)

	cc, err = distr.GetConn(cc.ID)
	require.Nil(t, err)
	assert.Equal(t, conn.Conn{}, cc)
}

func TestPubSubUnsub(t *T) {
	ch := testutil.RandStr()
	cb, prb := testConn(true, ch)
	time.Sleep(100 * time.Millisecond)
	c1, pr1 := testConn(false, ch)
	c2, pr2 := testConn(false, ch)

	assertPubEqual := func(typ distr.PubType, from string, msg string, p distr.Pub) {
		assert.Equal(t, typ, p.Type)
		if from == "backend" {
			assert.True(t, p.Conn.IsBackend)
		} else {
			assert.Equal(t, from, p.Conn.Presence)
		}
		assert.Equal(t, ch, p.Channel)

		if msg != "" {
			b, _ := json.Marshal(msg)
			msgj := json.RawMessage(b)
			assert.Equal(t, &msgj, p.Message)
		}
	}

	//////////////////////////
	// Make sure backend receives sub messages from both clients

	var pb distr.Pub

	for i := 0; i < 2; i++ {
		pb = distr.Pub{}
		requireRcv(t, cb, &pb)
		assert.True(t, pb.Conn.Presence == pr1 || pb.Conn.Presence == pr2)
		pb.Conn.Presence = ""
		assertPubEqual(distr.PubTypeSub, "", "", pb)
	}

	//////////////////////////
	// Publish from client, backend should get it

	msg := testutil.RandStr()
	testPub(pr1, msg, ch)

	pb = distr.Pub{}
	requireRcv(t, cb, &pb)
	assertPubEqual(distr.PubTypePub, pr1, msg, pb)

	//////////////////////////
	// Publish from backend, both clients should get it

	msg = testutil.RandStr()
	testPub(prb, msg, ch)

	var p1 distr.Pub
	requireRcv(t, c1, &p1)
	assertPubEqual(distr.PubTypePub, prb, msg, p1)

	var p2 distr.Pub
	requireRcv(t, c2, &p2)
	assertPubEqual(distr.PubTypePub, prb, msg, p2)

	//////////////////////////
	// Close c1, then publish from backend, only c2 should get it (duh). Also
	// check for unsub message

	c1.Close()

	pb = distr.Pub{}
	requireRcv(t, cb, &pb)
	assertPubEqual(distr.PubTypeUnsub, pr1, "", pb)

	msg = testutil.RandStr()
	testPub(prb, msg, ch)

	p2 = distr.Pub{}
	requireRcv(t, c2, &p2)
	assertPubEqual(distr.PubTypePub, prb, msg, p2)
}
