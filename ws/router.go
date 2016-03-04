package ws

import "sync"

// rConn contains the information about a connection which can be stored in the
// router. The rConn value for a connection can never change for the life of the
// connection.
type rConn struct {
	// will be closed when the connection is closed. Anything writing to other
	// channels in this struct should also select on this, in case the
	// connection closed in between grabbing this struct and writing to that
	// channel
	closeCh chan struct{}
}

var r = map[string]rConn{}
var rlock sync.RWMutex

func getRConn(id string) (rConn, bool) {
	rlock.RLock()
	rc, ok := r[id]
	rlock.RUnlock()
	return rc, ok
}
