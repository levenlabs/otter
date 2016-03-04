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

// NewHandler returns an http.Handler which handles the websocket interface
func NewHandler() http.Handler {
	return websocket.Server{Handler: handler}
}

func handler(c *websocket.Conn) {
	ws := wsConn{
		Conn: conn.New(),
		rConn: rConn{
			closeCh: make(chan struct{}),
		},
		c:      c,
		enc:    json.NewEncoder(c),
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
	c   *websocket.Conn
	enc *json.Encoder

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

func (ws *wsConn) writeError(m json.RawMessage, err error) {
	ws.enc.Encode(Error{
		Error: err,
		From:  m,
	})
}

type cmdInfo struct {
	m         json.RawMessage
	isBackend bool
}

func (ws *wsConn) cmd(m json.RawMessage) {
	var cmdBase Command
	if err := json.Unmarshal(m, &cmdBase); err != nil {
		ws.writeError(m, err)
		return
	}

	ci := cmdInfo{
		m: m,
	}

	if cmdBase.Signature != "" && Auth.Verify(cmdBase.Signature, ws.ID) {
		ci.isBackend = true
	}

	switch cmdBase.Command {
	case "echo":
		ws.cmdEcho(ci)
	case "auth":
		ws.cmdAuth(ci)
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
		ws.writeError(ci.m, err)
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
		ws.writeError(ci.m, err)
		return
	} else if !Auth.Verify(args.Signature, args.Presence) {
		ws.writeError(ci.m, errors.New("invalid signature"))
		return
	}

	ws.Conn.Presence = args.Presence
	ws.log(llog.Info, "auth cmd", nil)

	if err := distr.SetConn(ws.Conn, connSetTimeout); err != nil {
		ws.log(llog.Error, "error setting cron (cmd)", llog.KV{"err": err})
		ws.writeError(ci.m, err)
		return
	}
}
