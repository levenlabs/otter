// Package ws handles the websocket interface of otter
package ws

import (
	"encoding/json"
	"net/http"

	"github.com/levenlabs/go-llog"
	"github.com/levenlabs/otter/conn"
	"golang.org/x/net/websocket"
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
		},
		c:      c,
		enc:    json.NewEncoder(c),
		readCh: make(chan json.RawMessage),
	}
	kv := llog.KV{
		"id": ws.ID,
		// TODO this panics for some reason
		//"remoteAddr": c.RemoteAddr().String(),
	}
	llog.Debug("conn created", kv)

	ws.start()
	kv["presence"] = ws.Presence
	llog.Debug("conn closed", kv)

	c.Close()
}

type wsConn struct {
	conn.Conn
	rConn
	c   *websocket.Conn
	enc *json.Encoder

	readCh chan json.RawMessage
}

func (ws *wsConn) start() {
	rlock.Lock()
	r[ws.ID] = ws.rConn
	rlock.Unlock()

	go ws.readLoop()
	ws.enc.Encode(ws.Conn)
loop:
	for {
		select {
		case m, ok := <-ws.readCh:
			if !ok {
				break loop
			}
			ws.cmd(m)
		}
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

func (ws *wsConn) cmd(m json.RawMessage) {
	var cmdBase Command
	if err := json.Unmarshal(m, &cmdBase); err != nil {
		ws.writeError(m, err)
		return
	}

	// TODO check cmdBase.Signature

	switch cmdBase.Command {
	case "echo":
		ws.cmdEcho(m)
	}
}

////////////////////////////////////////////////////////////////////////////////

// CommandEcho simply echos back the given message. Useful for testing
type CommandEcho struct {
	Command
	Message string `json:"message"`
}

func (ws *wsConn) cmdEcho(m json.RawMessage) {
	var res CommandEcho
	if err := json.Unmarshal(m, &res); err != nil {
		ws.writeError(m, err)
		return
	}
	ws.enc.Encode(res)
}
