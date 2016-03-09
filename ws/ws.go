// Package ws handles the websocket interface of otter
package ws

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/levenlabs/go-llog"
	"github.com/levenlabs/otter/auth"
	"github.com/levenlabs/otter/conn"
	"github.com/levenlabs/otter/distr"
	"golang.org/x/net/websocket"
)

var connSetTimeout = 30 * time.Second

// Needs to be set in order to properly handle authentication
var Auth auth.Auth

var (
	errInvalidSig = errors.New("invalid signature")
)

// NewHandler returns an http.Handler which handles the websocket interface
func NewHandler() http.Handler {
	return websocket.Server{Handler: handler}
}

func handler(c *websocket.Conn) {
	ws := wsConn{
		Conn: conn.New(),
		rConn: rConn{
			closeCh: make(chan struct{}),
			pubCh:   make(chan distr.Pub, 10),
		},
		c:      c,
		enc:    json.NewEncoder(c),
		subs:   map[string]struct{}{},
		readCh: make(chan json.RawMessage),
	}
	ws.log(llog.Debug, "conn created", nil)

	ws.start()
	ws.log(llog.Debug, "conn closed", nil)

	c.Close()
}

type wsConn struct {
	conn.Conn
	rConn
	c    *websocket.Conn
	enc  *json.Encoder
	subs map[string]struct{}

	readCh chan json.RawMessage
}

func (ws *wsConn) log(fn llog.LogFunc, msg string, kv llog.KV) {
	akv := llog.KV{
		"id": ws.ID,
		// TODO this panics for some reason
		//"remoteAddr": c.RemoteAddr().String(),
	}
	if ws.Presence != "" {
		akv["presence"] = ws.Presence
	}
	if ws.Conn.IsBackend {
		akv["backend"] = true
	}
	for k, v := range kv {
		akv[k] = v
	}
	fn(msg, akv)
}

func (ws *wsConn) start() {
	rlock.Lock()
	r[ws.ID] = ws.rConn
	rlock.Unlock()

	connSetTick := time.NewTicker(connSetTimeout / 4)
	defer connSetTick.Stop()

	go ws.readLoop()

	if err := distr.SetConn(ws.Conn, connSetTimeout); err != nil {
		ws.log(llog.Error, "error setting conn (init)", llog.KV{"err": err})
		// We could theoretically exit here, but the conn data will be set at
		// some point, so might as well just wait for it
	}

	ws.enc.Encode(ws.Conn)
loop:
	for {
		select {
		case m, ok := <-ws.readCh:
			if !ok {
				break loop
			}
			ws.cmd(m)

		case <-connSetTick.C:
			if err := distr.SetConn(ws.Conn, connSetTimeout); err != nil {
				ws.log(llog.Error, "error setting conn", llog.KV{"err": err})
			}

		case p := <-ws.rConn.pubCh:
			ws.enc.Encode(p)
		}
	}

	for ch := range ws.subs {
		if err := ws.unsub(ch); err != nil {
			ws.log(llog.Error, "error unsubbing during teardown", llog.KV{
				"channel": ch,
				"err":     err,
			})
		}
	}

	if err := distr.UnsetConn(ws.Conn); err != nil {
		ws.log(llog.Error, "error unsetting conn", llog.KV{"err": err})
	}
	rlock.Lock()
	close(ws.closeCh)
	delete(r, ws.ID)
	rlock.Unlock()
}

func (ws *wsConn) readLoop() {
	dec := json.NewDecoder(ws.c)
	defer func() { close(ws.readCh) }()

	for {
		var m json.RawMessage
		if err := dec.Decode(&m); err != nil {
			return
		}

		ws.readCh <- m
	}
}

// Command is used in all commands sent to otter. It includes the name of the
// command, and optionally a signature of the connection id if the command is
// coming from the backend application
type Command struct {
	Command   string `json:"command"`
	Signature string `json:"signature,omitempty"`
}

// Error is pushed to the connection when something unexpected has occurred
type Error struct {
	Error error           `json:"error"`
	From  json.RawMessage `json:"from"`
}

func (ws *wsConn) writeError(m json.RawMessage, logMsg string, err error) {
	ws.enc.Encode(Error{
		Error: err,
		From:  m,
	})
	if logMsg != "" {
		ws.log(llog.Error, logMsg, llog.KV{"err": err})
	}
}

type cmdInfo struct {
	m json.RawMessage
}

func (ws *wsConn) cmd(m json.RawMessage) {
	var cmdBase Command
	if err := json.Unmarshal(m, &cmdBase); err != nil {
		ws.writeError(m, "", err)
		return
	}

	ci := cmdInfo{
		m: m,
	}

	switch cmdBase.Command {
	case "echo":
		ws.cmdEcho(ci)
	case "auth":
		ws.cmdAuth(ci)
	case "pub":
		ws.cmdPub(ci)
	case "sub":
		ws.cmdSub(ci)
	case "unsub":
		ws.cmdUnsub(ci)
	}
}

////////////////////////////////////////////////////////////////////////////////

// CommandEcho simply echos back the given message. Useful for testing
type CommandEcho struct {
	Command
	Message string `json:"message"`
}

func (ws *wsConn) cmdEcho(ci cmdInfo) {
	var res CommandEcho
	if err := json.Unmarshal(ci.m, &res); err != nil {
		ws.writeError(ci.m, "", err)
		return
	}
	ws.log(llog.Info, "echo cmd", llog.KV{"message": res.Message})
	ws.enc.Encode(res)
}

////////////////////////////////////////////////////////////////////////////////

// CommandAuth is used to assign a "presence" string to a connection, so that it
// can identify itself to other clients
type CommandAuth struct {
	Command
	Presence  string `json:"presence"`
	Signature string `json:"signature"`
}

func (ws *wsConn) cmdAuth(ci cmdInfo) {
	var args CommandAuth
	if err := json.Unmarshal(ci.m, &args); err != nil {
		ws.writeError(ci.m, "", err)
		return
	}

	if Auth.Verify(args.Signature, string(ws.Conn.ID)) {
		if len(ws.subs) > 0 {
			ws.writeError(ci.m, "", errors.New("cannot become backend with active subs"))
		}
		ws.Conn.IsBackend = true
	} else if !Auth.Verify(args.Signature, args.Presence) {
		ws.writeError(ci.m, "", errInvalidSig)
		return
	}

	ws.Conn.Presence = args.Presence
	ws.log(llog.Info, "auth cmd", nil)

	if err := distr.SetConn(ws.Conn, connSetTimeout); err != nil {
		ws.writeError(ci.m, "error setting conn (cmd)", err)
		return
	}
}

////////////////////////////////////////////////////////////////////////////////

// CommandAuth is used to publish a message to one or more channels
type CommandPub struct {
	Command
	Channel string           `json:"channel"`
	Message *json.RawMessage `json:"message"`
}

func (ws *wsConn) cmdPub(ci cmdInfo) {
	var args CommandPub
	if err := json.Unmarshal(ci.m, &args); err != nil {
		ws.writeError(ci.m, "", err)
		return
	}
	ws.log(llog.Debug, "pub", llog.KV{"ch": args.Channel})

	err := distr.Publish(distr.Pub{
		Type:    distr.PubTypePub,
		Conn:    ws.Conn,
		Channel: args.Channel,
		Message: args.Message,
	})
	if err != nil {
		ws.writeError(ci.m, "error publishing", err)
		return
	}
}

////////////////////////////////////////////////////////////////////////////////

type CommandSub struct {
	Command
	Channel string `json:"channel"`
}

func (ws *wsConn) cmdSub(ci cmdInfo) {
	var args CommandSub
	if err := json.Unmarshal(ci.m, &args); err != nil {
		ws.writeError(ci.m, "", err)
		return
	}
	ws.log(llog.Debug, "sub", llog.KV{"ch": args.Channel})

	if err := distr.Subscribe(ws.Conn, args.Channel); err != nil {
		ws.writeError(ci.m, "error subscribing", err)
		return
	}

	if _, ok := ws.subs[args.Channel]; !ok && !ws.Conn.IsBackend {
		if err := distr.Publish(distr.Pub{
			Type:    distr.PubTypeSub,
			Conn:    ws.Conn,
			Channel: args.Channel,
		}); err != nil {
			ws.log(llog.Error, "error publishing subcribe", llog.KV{
				"ch":  args.Channel,
				"err": err,
			})
		}
	}

	ws.subs[args.Channel] = struct{}{}
}

////////////////////////////////////////////////////////////////////////////////

type CommandUnsub struct {
	Command
	Channel string `json:"channel"`
}

func (ws *wsConn) unsub(ch string) error {
	if err := distr.Unsubscribe(ws.Conn, ch); err != nil {
		return err
	}

	if _, ok := ws.subs[ch]; ok && !ws.Conn.IsBackend {
		if err := distr.Publish(distr.Pub{
			Type:    distr.PubTypeUnsub,
			Conn:    ws.Conn,
			Channel: ch,
		}); err != nil {
			ws.log(llog.Error, "error publishing unsubcribe", llog.KV{
				"ch":  ch,
				"err": err,
			})
		}
	}

	delete(ws.subs, ch)
	return nil
}

func (ws *wsConn) cmdUnsub(ci cmdInfo) {
	var args CommandUnsub
	if err := json.Unmarshal(ci.m, &args); err != nil {
		ws.writeError(ci.m, "", err)
		return
	}
	ws.log(llog.Debug, "unsub", llog.KV{"ch": args.Channel})

	if err := ws.unsub(args.Channel); err != nil {
		ws.writeError(ci.m, "error unsubscribing", err)
		return
	}
}
