// Package conn contains a simple datastructure and some general purpose methods
// for interacting with it
package conn

import (
	"encoding/hex"
	"strings"

	"github.com/satori/go.uuid"
)

// NodeID is the unique identifier for this node. It should be set before New
// connections are generated
var NodeID string

const idSep = "_"

// Conn represents all the information needed to be stored about a connection
type Conn struct {
	ID       string `json:"id"`
	Presence string `json:"presence,omitempty"`
}

// New returns a new Conn object with a brand new ID
func New() Conn {
	baseID := hex.EncodeToString(uuid.NewV4().Bytes())
	return Conn{
		ID: NodeID + idSep + baseID,
	}
}

// NodeID returns the id of the node that this connection was created on
func (c Conn) NodeID() string {
	p := strings.SplitN(c.ID, idSep, 2)
	if len(p) != 2 {
		return ""
	}
	return p[0]
}
