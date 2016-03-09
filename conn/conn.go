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

// ID is used to identify a single connection across all instances of otter in a
// cluster
type ID string

// NodeID returns the id of the node that this ID was created on
func (id ID) NodeID() string {
	p := strings.SplitN(string(id), idSep, 2)
	if len(p) != 2 {
		return ""
	}
	return p[0]
}

// Conn represents all the information needed to be stored about a connection
type Conn struct {
	ID        ID     `json:"id"`
	Presence  string `json:"presence,omitempty"`
	IsBackend bool   `json:"isBackend,omitempty"`
}

// New returns a new Conn object with a brand new ID
func New() Conn {
	baseID := hex.EncodeToString(uuid.NewV4().Bytes())
	return Conn{
		ID: ID(NodeID + idSep + baseID),
	}
}
