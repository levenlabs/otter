package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"

	"github.com/levenlabs/go-llog"
	"github.com/levenlabs/otter/conn"
	"github.com/levenlabs/otter/distr"
	"github.com/levenlabs/otter/ws"
	"github.com/mediocregopher/lever"
)

func main() {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	conn.NodeID = hex.EncodeToString(b)

	l := lever.New("otter", nil)
	l.Add(lever.Param{
		Name:        "--ws-url",
		Description: "Address and URL the websocket interface should listen on. Only http is currently supported",
		Default:     "http://:4444/subs",
	})
	l.Add(lever.Param{
		Name:        "--auth-secret",
		Description: "secret key to use to verify connection presence information. Must be the same across all otter nodes and backend applications",
	})
	l.Add(lever.Param{
		Name:        "--redis-addr",
		Description: "Address of redis node to use. If node is in a cluster the rest o f the cluster will be discovered automatically",
		Default:     "127.0.0.1:6379",
	})
	l.Add(lever.Param{
		Name:        "--redis-pool-size",
		Description: "Number of connections to make per-redis instance",
		Default:     "10",
	})
	l.Add(lever.Param{
		Name:        "--redis-num-sub-conns",
		Description: "Number of connections to make to subscribe to publishes being broadcast across the cluster. This number must be consistent across all otter nodes",
		Default:     "10",
	})
	l.Parse()

	secret, _ := l.ParamStr("--auth-secret")
	if secret == "" {
		llog.Fatal("--auth-secret is required")
	}

	redisAddr, _ := l.ParamStr("--redis-addr")
	redisPoolSize, _ := l.ParamInt("--redis-pool-size")
	redisNumSubConns, _ := l.ParamInt("--redis-num-sub-conns")

	wsURLRaw, _ := l.ParamStr("--ws-url")
	wsURL, err := url.Parse(wsURLRaw)
	if err != nil {
		llog.Fatal("could not parse --ws-url", llog.KV{
			"wsURL": wsURLRaw,
			"err":   err,
		})
	}
	if wsURL.Path == "" {
		wsURL.Path = "/"
	}

	distr.Init(redisAddr, redisPoolSize, redisNumSubConns)
	ws.Init(secret, redisNumSubConns)

	http.Handle(wsURL.Path, http.StripPrefix(wsURL.Path, ws.NewHandler()))
	llog.Info("websocket interface listening", llog.KV{"addr": wsURL})
	err = http.ListenAndServe(wsURL.Host, nil)
	llog.Fatal("websocket interface failed", llog.KV{"addr": wsURL, "err": err})
}
