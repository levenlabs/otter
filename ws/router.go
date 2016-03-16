package ws

import (
	"sync"
	"time"

	"github.com/levenlabs/go-llog"
	"github.com/levenlabs/otter/conn"
	"github.com/levenlabs/otter/distr"
)

// rConn contains the information about a connection which can be stored in the
// router. The rConn value for a connection can never change for the life of the
// connection.
type rConn struct {
	// will be closed when the connection is closed. Anything writing to other
	// channels in this struct should also select on this, in case the
	// connection closed in between grabbing this struct and writing to that
	// channel
	closeCh chan struct{}

	pubCh chan distr.Pub
}

var r = map[conn.ID]rConn{}
var rlock sync.RWMutex

func getRConn(id conn.ID) (rConn, bool) {
	rlock.RLock()
	rc, ok := r[id]
	rlock.RUnlock()
	return rc, ok
}

func routerInit(numReaders int) {
	llog.Info("starting PubCh readers", llog.KV{"numReaders": numReaders})
	for i := 0; i < numReaders; i++ {
		go pubReader(i)
	}
	go cleanup()
}

func pubReader(i int) {
	for p := range distr.PubCh {
		kv := llog.KV{"ch": p.Channel, "i": i}

		ids, err := distr.GetSubscribed(conn.NodeID, p.Channel, !p.Conn.IsBackend, connSetTimeout)
		if err != nil {
			kv["err"] = err
			llog.Error("error getting subscribed", kv)
			continue
		}

		for _, id := range ids {
			rc, ok := getRConn(id)
			if !ok {
				continue
			}

			select {
			case rc.pubCh <- p:
			case <-rc.closeCh:
			default:
				kv["id"] = id
				llog.Error("pubCh buffer full", kv)
			}
		}
	}
}

func cleanup() {
	for range time.Tick(connSetTimeout / 2) {
		distr.CleanChannels(false, connSetTimeout)
		distr.CleanChannels(true, connSetTimeout)
	}
}
