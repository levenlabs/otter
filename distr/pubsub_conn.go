package distr

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/levenlabs/go-llog"
	"github.com/levenlabs/otter/conn"
	"github.com/mediocregopher/radix.v2/pubsub"
	"github.com/mediocregopher/radix.v2/redis"
)

func initSubs(addr string, count int) {
	numSubKeys = count

	for i := 0; i < count; i++ {
		go spinSub(addr, i)
	}
}

var numSubKeys int

func subKey(i int) string {
	return fmt.Sprintf("sub:%d", i)
}

func randSubKey() string {
	return subKey(rand.Intn(numSubKeys))
}

// Pub describes a publish message either being sent out to other nodes or being
// received by this one
type Pub struct {
	Conn    conn.Conn
	Channel string
	Message *json.RawMessage
}

// PubCh is where publishes which are being received by this node are written
// to. They need to be read off the channel and consumed constantly
var PubCh = make(chan Pub, 1000)

func spinSub(addr string, i int) {
	var c *redis.Client
	var err error

	for {
		if c != nil {
			c.Close()
			time.Sleep(1 * time.Second)
		}

		kv := llog.KV{
			"addr": addr,
			"i":    i,
		}

		llog.Info("starting sub connection", kv)
		c, err = redis.Dial("tcp", addr)
		if err != nil {
			kv["err"] = err
			llog.Error("could not start sub connection", kv)
			continue
		}

		subc := pubsub.NewSubClient(c)
		if err := subc.Subscribe(subKey(i)).Err; err != nil {
			kv["err"] = err
			llog.Error("could not subscribe", kv)
			continue
		}

		for {
			r := subc.Receive()
			if r.Timeout() {
				continue
			} else if r.Err != nil {
				kv["err"] = r.Err
				llog.Error("error receiving on sub connection", kv)
				break
			}

			var p Pub
			if err := json.Unmarshal([]byte(r.Message), &p); err != nil {
				kv["err"] = r.Err
				llog.Error("error decoding pub", kv)
				break
			}

			PubCh <- p
		}
	}
}

// Publish sends the given Pub struct to all listening otter instances,
// including this one
func Publish(p Pub) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}

	return cmder.Cmd("PUBLISH", randSubKey(), b).Err
}
