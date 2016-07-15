// Package ws handles the websocket interface of otter
package ws

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/levenlabs/go-llog"
	"github.com/levenlabs/otter/auth"
	"github.com/levenlabs/otter/conn"
	"github.com/levenlabs/otter/distr"
	"golang.org/x/net/websocket"
)

var connSetTimeout = 30 * time.Second

// Auth needs to be set in order to properly handle authentication
var Auth auth.Auth

var (
	errInvalidSig = errors.New("invalid signature")
)

// Init initializes connection routing
func Init(secret string, numReaders int) {
	Auth.Key = secret
	routerInit(numReaders)
}

// NewHandler returns an http.Handler which handles the websocket interface
func NewHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		} else if r.URL.Path[0] == '/' {
			r.URL.Path = r.URL.Path[1:]
		}

		if r.Method == "POST" {
			pubHandler(w, r)
		} else if r.Method == "GET" {
			getHandler(w, r)
		} else {
			http.Error(w, "invalid method", http.StatusMethodNotAllowed)
		}
	})
}

func getConnInfo(r *http.Request) (conn.Conn, []string, error) {
	p := strings.SplitN(r.URL.Path, "/", 2)[0]
	subs := strings.Split(p, ",")
	subsF := subs[:0]
	for _, sub := range subs {
		if sub != "" {
			subsF = append(subsF, sub)
		}
	}

	c := conn.New()
	presence, sig := r.FormValue("presence"), r.FormValue("sig")
	if presence != "" && !Auth.Verify(sig, presence) {
		return c, subs, errInvalidSig
	}
	if presence == "backend" {
		c.IsBackend = true
	} else if presence != "" {
		c.Presence = presence
	}
	return c, subs, nil
}

func pubHandler(w http.ResponseWriter, r *http.Request) {
	c, subs, err := getConnInfo(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	var msg json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	for _, ch := range subs {
		err := distr.Publish(distr.Pub{
			Type:    "pub",
			Conn:    c,
			Channel: ch,
			Message: &msg,
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			llog.Error("publish failed", llog.KV{
				"presence": c.Presence,
				"channel":  ch,
				"err":      err,
			})
		}
	}
}

func listSubbed(chs ...string) ([]conn.Conn, error) {
	nIDs, err := distr.GetNodeIDs()
	if err != nil {
		return nil, err
	}

	m := map[conn.Conn]struct{}{}
	for _, nID := range nIDs {
		for _, ch := range chs {
			cc, err := distr.GetSubscribed(nID, ch, false, connSetTimeout)
			if err != nil {
				return nil, err
			}
			for _, c := range cc {
				m[c] = struct{}{}
			}
		}
	}

	res := make([]conn.Conn, 0, len(m))
	for c := range m {
		res = append(res, c)
	}
	return res, nil
}

// SubListRes is the structur that the list of connection objects from a call to
// /subbed will be returned in
type SubListRes struct {
	Conns []conn.Conn `json:"conns"`
}

func getHandler(w http.ResponseWriter, r *http.Request) {
	pp := strings.SplitN(r.URL.Path, "/", 2)
	var suffix string
	if len(pp) == 2 {
		suffix = pp[1]
	}

	if suffix == "" {
		(websocket.Server{Handler: handler}).ServeHTTP(w, r)

	} else if suffix == "subbed" {
		cc, subs, err := getConnInfo(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		} else if !cc.IsBackend {
			http.Error(w, "not allowed", http.StatusForbidden)
		}

		conns, err := listSubbed(subs...)
		if err != nil {
			llog.Error("error getting subbed connections", llog.KV{"subs": subs, "err": err})
			http.Error(w, err.Error(), http.StatusForbidden)
		}

		json.NewEncoder(w).Encode(SubListRes{Conns: conns})
	}
}

type wsConn struct {
	conn.Conn
	rConn
	c           *websocket.Conn
	enc         *json.Encoder
	subs        []string
	connCloseCh chan struct{}
}

func newWSConn(c *websocket.Conn) (wsConn, error) {
	ws := wsConn{
		rConn: rConn{
			closeCh: make(chan struct{}),
			pubCh:   make(chan distr.Pub, 10),
		},
		c:           c,
		enc:         json.NewEncoder(c),
		connCloseCh: make(chan struct{}),
	}

	cc, subs, err := getConnInfo(c.Request())
	if err != nil {
		return ws, err
	}
	ws.Conn = cc
	ws.subs = subs

	ws.log(llog.Debug, "conn created", nil)
	return ws, nil
}

func handler(c *websocket.Conn) {
	ws, err := newWSConn(c)
	if err != nil {
		ws.writeError("", err, nil)
		return
	}

	rlock.Lock()
	r[ws.ID] = ws.rConn
	rlock.Unlock()

	defer func() {
		rlock.Lock()
		close(ws.closeCh)
		delete(r, ws.ID)
		rlock.Unlock()

		ws.log(llog.Debug, "conn closed", nil)
		ws.c.Close()
	}()

	if err := distr.SetConn(ws.Conn, connSetTimeout); err != nil {
		ws.writeError("error setting conn (init)", err, nil)
		return
	}

	for _, ch := range ws.subs {
		kv := llog.KV{"channel": ch}
		if err := distr.Subscribe(ws.Conn, ch); err != nil {
			ws.writeError("error subscribing (init)", err, kv)
			return
		}
		if err := distr.Publish(distr.Pub{
			Type:    "sub",
			Conn:    ws.Conn,
			Channel: ch,
		}); err != nil {
			ws.writeError("error publishing subscribe", err, kv)
			return
		}
	}

	ws.spin()

	for _, ch := range ws.subs {
		if err := distr.Unsubscribe(ws.Conn, ch); err != nil {
			ws.log(llog.Error, "error unsubbing during teardown", llog.KV{
				"channel": ch,
				"err":     err,
			})
		}
		if err := distr.Publish(distr.Pub{
			Type:    "unsub",
			Conn:    ws.Conn,
			Channel: ch,
		}); err != nil {
			ws.log(llog.Error, "error publishing unsubcribe", llog.KV{
				"ch":  ch,
				"err": err,
			})
		}
	}

	if err := distr.UnsetConn(ws.Conn); err != nil {
		ws.log(llog.Error, "error unsetting conn", llog.KV{"err": err})
	}
}

func (ws *wsConn) spin() {
	go ws.readSpin()
	connSetTick := time.NewTicker(connSetTimeout / 4)
	defer connSetTick.Stop()

	for {
		select {
		case <-connSetTick.C:
			if err := distr.SetConn(ws.Conn, connSetTimeout); err != nil {
				ws.log(llog.Error, "error re-setting conn", llog.KV{"err": err})
			}
			for _, ch := range ws.subs {
				if err := distr.Subscribe(ws.Conn, ch); err != nil {
					ws.log(llog.Error, "error re-subscribing conn", llog.KV{
						"err":     err,
						"channel": ch,
					})
				}
			}

		case p := <-ws.rConn.pubCh:
			ws.enc.Encode(p)

		case <-ws.connCloseCh:
			return
		}
	}

}

// This is only used to determine if the connection has died, we don't actually
// ever read anything from the client
func (ws *wsConn) readSpin() {
	defer func() { close(ws.connCloseCh) }()

	b := make([]byte, 1)
	for {
		_, err := ws.c.Read(b)
		if nerr, ok := err.(*net.OpError); ok && nerr.Timeout() {
			continue
		} else if err != nil {
			return
		}
	}
}

func (ws *wsConn) log(fn llog.LogFunc, msg string, kv llog.KV) {
	akv := llog.KV{
		"id":         ws.ID,
		"subs":       ws.subs,
		"remoteAddr": ws.c.Request().RemoteAddr,
	}
	if ws.Presence != "" {
		akv["presence"] = ws.Presence
	}
	for k, v := range kv {
		akv[k] = v
	}
	fn(msg, akv)
}

// Error is pushed to the connection when something unexpected has occurred
type Error struct {
	Error error `json:"error"`
}

func (ws *wsConn) writeError(logMsg string, err error, kv llog.KV) {
	ws.enc.Encode(Error{
		Error: err,
	})
	kv2 := llog.KV{}
	for k, v := range kv {
		kv2[k] = v
	}
	kv2["err"] = err
	ws.log(llog.Error, logMsg, kv2)
}
