// Package distr handles storing data to redis and managing how it is
// distributed amongst the redis cluster. It also handles cleaning up old data
// from dead nodes
package distr

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/levenlabs/go-llog"
	"github.com/levenlabs/golib/radixutil"
	"github.com/levenlabs/otter/conn"
	"github.com/mediocregopher/radix.v2/cluster"
	"github.com/mediocregopher/radix.v2/pool"
	"github.com/mediocregopher/radix.v2/redis"
	"github.com/mediocregopher/radix.v2/util"
)

var cmder util.Cmder

func withConn(key string, fn func(*redis.Client)) error {
	switch ct := cmder.(type) {
	case *pool.Pool:
		conn, err := ct.Get()
		if err != nil {
			return err
		}
		fn(conn)
		ct.Put(conn)

	case *cluster.Cluster:
		conn, err := ct.GetForKey(key)
		if err != nil {
			return err
		}
		fn(conn)
		ct.Put(conn)
	}

	return nil
}

// Init initializes the shared util.Cmder instance connected to the given
// address (single instance or cluster), as well as sets up any necessary
// go-routines
func Init(addr string, poolSize int) {
	kv := llog.KV{
		"addr":     addr,
		"poolSize": poolSize,
	}

	var err error
	llog.Info("connecting to redis", kv)
	cmder, err = radixutil.DialMaybeCluster("tcp", addr, poolSize)
	if err != nil {
		kv["err"] = err
		llog.Fatal("error connecting to redis", kv)
	}
}

func connKey(cID string) string {
	return fmt.Sprintf("conn:%s", cID)
}

// SetConn sets the given connection's Conn struct into redis for the given
// amount of time
func SetConn(c conn.Conn, timeout time.Duration) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	milli := uint64(timeout / time.Millisecond)
	return cmder.Cmd("PSETEX", connKey(c.ID), milli, b).Err
}

// UnsetConn unsets the given connection's Conn struct immediately
func UnsetConn(c conn.Conn) error {
	return cmder.Cmd("DEL", connKey(c.ID)).Err
}

// GetConn returns the connection info for the connection with the given ID.
// Returns empty conn.Conn if the connection wasn't found
func GetConn(cID string) (conn.Conn, error) {
	var c conn.Conn
	r := cmder.Cmd("GET", connKey(cID))
	if r.Err != nil {
		return c, r.Err
	} else if r.IsType(redis.Nil) {
		return c, nil
	}

	b, err := r.Bytes()
	if err != nil {
		return c, nil
	}

	err = json.Unmarshal(b, &c)
	return c, err
}
