// Package conn contains a simple datastructure and some general purpose methods
// for interacting with it
package conn

import (
	"encoding/hex"
	"strings"

	"github.com/satori/go.uuid"
)

//go:generate msgp

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

// MarshalBinary takes returns a binary string representation of the Conn
func (c Conn) MarshalBinary() ([]byte, error) {
	return c.MarshalMsg(nil)
}

// UnmarshalBinary unmarshals a binary string returned from MarshalBinary into
// the Conn
func (c *Conn) UnmarshalBinary(b []byte) error {
	_, err := c.UnmarshalMsg(b)
	return err
}
