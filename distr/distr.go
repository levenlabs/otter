// Package distr handles storing data to redis and managing how it is
// distributed amongst the redis cluster. It also handles cleaning up old data
// from dead nodes
package distr

import (
	"encoding/json"
	"fmt"
	"strconv"
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
func Init(addr string, poolSize, subConnCount int) {
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

	initSubs(addr, subConnCount)
}

func connKey(cID conn.ID) string {
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
func GetConn(cID conn.ID) (conn.Conn, error) {
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

func channelKey(nodeID, channel string, isBackend bool) string {
	if isBackend {
		return fmt.Sprintf("channel:{%s}:backend:%s", nodeID, channel)
	}
	return fmt.Sprintf("channel:{%s}:%s", nodeID, channel)
}

// Subscribe adds the given connection to the set of connections subscribed to
// the channel. Backend connections get their own set.
func Subscribe(c conn.Conn, channel string) error {
	k := channelKey(c.ID.NodeID(), channel, c.IsBackend)
	return cmder.Cmd("ZADD", k, time.Now().UnixNano(), c.ID).Err
}

// Unsubscribe removes the given connection from the set of connections
// subscribed to the channel
func Unsubscribe(c conn.Conn, channel string) error {
	k := channelKey(c.ID.NodeID(), channel, c.IsBackend)
	return cmder.Cmd("ZREM", k, c.ID).Err
}

// GetSubscribed returns the set of connections on the given node which are
// subscribed to the given channel. Does not include backend connections.
func GetSubscribed(nodeID, channel string, backend bool, timeout time.Duration) ([]conn.ID, error) {
	k := channelKey(nodeID, channel, backend)
	tlower := time.Now().Add(-timeout).UnixNano()
	l, err := cmder.Cmd("ZRANGEBYSCORE", k, tlower, "+inf").List()
	if err != nil {
		return nil, err
	}

	cc := make([]conn.ID, len(l))
	for i := range l {
		cc[i] = conn.ID(l[i])
	}
	return cc, nil
}

// CleanChannels runs through all the channels in the cluster and removes
// entries from them that are older than the given timeout. Only operates on
// frontend/backend subs in a single call, so this will probably have to be
// called twice
func CleanChannels(backend bool, timeout time.Duration) {
	ch := make(chan string)
	var err error
	go func() {
		err = util.Scan(cmder, ch, "SCAN", "", channelKey("*", "*", backend))
	}()
	tupper := time.Now().Add(-timeout).UnixNano()
	tupperStr := "(" + strconv.FormatInt(tupper, 10)
	for k := range ch {
		cerr := cmder.Cmd("ZREMRANGEBYSCORE", k, "-inf", tupperStr).Err
		if cerr != nil {
			llog.Error("error cleaning channel", llog.KV{
				"key":     k,
				"backend": backend,
				"err":     cerr,
			})
		}
	}
	if err != nil {
		llog.Error("error scanning for channels to clean", llog.KV{
			"backend": backend,
			"err":     err,
		})
	}
}
