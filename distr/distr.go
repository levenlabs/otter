// Package distr handles storing data to redis and managing how it is
// distributed amongst the redis cluster. It also handles cleaning up old data
// from dead nodes
package distr

import (
	"fmt"
	"strconv"
	"strings"
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

func channelKeyPrefix(nodeID string) string {
	return fmt.Sprintf("channel:{%s}", nodeID)
}

func channelKey(nodeID, channel string, isBackend bool) string {
	if isBackend {
		return fmt.Sprintf("%s:backend:%s", channelKeyPrefix(nodeID), channel)
	}
	return fmt.Sprintf("%s:%s", channelKeyPrefix(nodeID), channel)
}

func channelKeyExtractNodeID(key string) string {
	a, b := strings.Index(key, "{"), strings.Index(key, "}")
	if a < 0 || b < 0 {
		return ""
	}
	return key[a+1 : b]
}

// Subscribe adds the given connection to the set of connections subscribed to
// the channel. Backend connections get their own set.
func Subscribe(c conn.Conn, channel string) error {
	b, err := c.MarshalBinary()
	if err != nil {
		return err
	}
	k := channelKey(c.ID.NodeID(), channel, c.IsBackend)
	return cmder.Cmd("ZADD", k, time.Now().UnixNano(), b).Err
}

// Unsubscribe removes the given connection from the set of connections
// subscribed to the channel
func Unsubscribe(c conn.Conn, channel string) error {
	b, err := c.MarshalBinary()
	if err != nil {
		return err
	}
	k := channelKey(c.ID.NodeID(), channel, c.IsBackend)
	return cmder.Cmd("ZREM", k, b).Err
}

// GetSubscribed returns the set of connections on the given node which are
// subscribed to the given channel. Does not include backend connections.
func GetSubscribed(nodeID, channel string, backend bool, timeout time.Duration) ([]conn.Conn, error) {
	k := channelKey(nodeID, channel, backend)
	tlower := time.Now().Add(-timeout).UnixNano()
	l, err := cmder.Cmd("ZRANGEBYSCORE", k, tlower, "+inf").ListBytes()
	if err != nil {
		return nil, err
	}

	cc := make([]conn.Conn, len(l))
	for i := range l {
		var c conn.Conn
		if err := c.UnmarshalBinary(l[i]); err != nil {
			return nil, err
		}
		cc[i] = c
	}
	return cc, nil
}

// CleanChannels runs through all the channels in the cluster and removes
// entries from them that are older than the given timeout. Only operates on
// frontend/backend subs in a single call, so this will probably have to be
// called twice
func CleanChannels(backend bool, timeout time.Duration) {
	tupper := time.Now().Add(-timeout).UnixNano()
	tupperStr := "(" + strconv.FormatInt(tupper, 10)

	it := util.NewScanner(cmder, util.ScanOpts{
		Command: "SCAN",
		Pattern: channelKey("*", "*", backend),
	})
	for it.HasNext() {
		k := it.Next()
		cerr := cmder.Cmd("ZREMRANGEBYSCORE", k, "-inf", tupperStr).Err
		if cerr != nil {
			llog.Error("error cleaning channel", llog.KV{
				"key":     k,
				"backend": backend,
				"err":     cerr,
			})
		}
	}
	if err := it.Err(); err != nil {
		llog.Error("error scanning for channels to clean", llog.KV{
			"backend": backend,
			"err":     err,
		})
	}
}

// GetNodeIDs returns the IDs of all the currently active nodes. This does a
// full key scan, so it shouldn't be used in any tight loops.
func GetNodeIDs() ([]string, error) {
	// TODO would be nice to have a lua wrapper which simply sets into a sorted
	// set every time a node does anything

	it := util.NewScanner(cmder, util.ScanOpts{
		Command: "SCAN",
		Pattern: channelKeyPrefix("*") + "*",
	})
	m := map[string]struct{}{}
	for it.HasNext() {
		k := it.Next()
		nID := channelKeyExtractNodeID(k)
		if nID == "" {
			continue
		}
		m[nID] = struct{}{}
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	res := make([]string, 0, len(m))
	for nID := range m {
		res = append(res, nID)
	}
	return res, nil
}
