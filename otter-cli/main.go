package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/levenlabs/otter/auth"
	"github.com/levenlabs/otter/go-otter"
	"github.com/mediocregopher/lever"
)

func fatalf(str string, args ...interface{}) {
	fmt.Printf(str+"\n", args...)
	os.Stdout.Sync()
	os.Stderr.Sync()
	os.Exit(1)
}

func main() {
	l := lever.New("otter-cli", &lever.Opts{
		DisallowConfigFile: true,
	})
	l.Add(lever.Param{
		Name:        "--otter-url",
		Description: "URL of otter instance to connect to",
		Default:     "ws://localhost:4444/subs",
	})
	l.Add(lever.Param{
		Name:        "--auth-secret",
		Description: "secret key to use to sign --presence. Must be as the auth secret of the otter instance being connected to. Required if --presence or --backend is set.",
	})
	l.Add(lever.Param{
		Name:        "--presence",
		Description: "presence JSON string to use. --auth-secret is required if this is set",
	})
	l.Add(lever.Param{
		Name:        "--backend",
		Description: "If set, this will be treated as a backend application connection. --auth-secret is required if this is set",
		Flag:        true,
	})
	l.Add(lever.Param{
		Name:        "--channel",
		Aliases:     []string{"-c"},
		Description: "Name of channel to act on. May be specified more than once for commands that allow interacting with multiple channels at the same time (e.g. --sub, --pub)",
	})
	l.Add(lever.Param{
		Name:        "--sub",
		Description: "Subscribes to the channels given and dumps all json subsequently sent",
		Flag:        true,
	})
	l.Add(lever.Param{
		Name:        "--pub",
		Description: "Publishes the given json string to all the channels given",
	})
	l.Parse()

	ustr, _ := l.ParamStr("--otter-url")
	u, err := url.Parse(ustr)
	if err != nil {
		fatalf("invalid --otter-url: %s", err)
	}

	c := otter.Client{
		URLs: []string{u.String()},
	}

	presence, _ := l.ParamStr("--presence")
	if backend := l.ParamFlag("--backend"); backend {
		presence = "backend"
	}
	if presence != "" {
		sec, _ := l.ParamStr("--auth-secret")
		if sec == "" {
			fatalf("--auth-secret required with --presence and --backend")
		}
		c.PresenceFunc = func() (string, string, error) {
			return presence, (auth.Auth{Key: sec}).Sign(presence), nil
		}
	}

	chs, _ := l.ParamStrs("--channel")
	if len(chs) == 0 {
		fatalf("at least one --channel (-c) required")
	}

	if l.ParamFlag("--sub") {
		pubCh := make(chan otter.Pub)
		errCh := c.Subscribe(pubCh, nil, chs...)
		go func() {
			fatalf("error reading from otter: %s", <-errCh)
		}()
		for p := range pubCh {
			b, err := json.Marshal(p)
			if err != nil {
				fatalf("error marshalling publish: %s", err)
			}
			fmt.Println(string(b))
		}
	}

	if msgStr, _ := l.ParamStr("--pub"); msgStr != "" {
		var msg interface{}
		if err := json.Unmarshal([]byte(msgStr), &msg); err != nil {
			fatalf("error unmarshalling --pub: %s", err)
		}
		if err := c.Publish(msg, chs...); err != nil {
			fatalf("error publishing: %s", err)
		}
		return
	}

	fatalf("--sub or --pub must be given")
}
