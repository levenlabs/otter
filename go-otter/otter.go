// Package otter is a go client for interacting with one or more otter servers.
//
// Subscribing
//
//	c := otter.Client{
//		Addrs: []string{"127.0.0.1:4444"},
//	}
//
//	pubCh := make(chan otter.Pub)
//	go func() {
//		for p := range pubCh {
//			log.Printf("got pub: %#v", p)
//		}
//	}()
//
//	for {
//		err := c.Subscribe(pubCh, nil, "someChannel")
//		log.Printf("got error subscribing: %s", err)
//		log.Printf("reconnecting")
//	}
//
// Backend application client
//
//	c := otter.Client{
//		Addrs:        []string{"127.0.0.1:4444"},
//		PresenceFunc: otter.BackendPresence("secret key"),
//	}
//
package otter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"

	"golang.org/x/net/websocket"

	"github.com/levenlabs/go-srvclient"
	"github.com/levenlabs/otter/auth"
	"github.com/levenlabs/otter/distr"
)

// Client is used to connect and interact with otter servers. The only required
// field is Addrs
type Client struct {
	// Addresses of otter instances. These will be picked from randomly when
	// making connections to otter. This field should not be changed while there
	// are active connections
	Addrs []string

	// Used to generate presence strings for connections made by this client.
	// This function will be called on every new connection made. If nil, no
	// presence information is ever used
	PresenceFunc

	// Set to true if otter is using https
	HTTPS bool
}

// Pub describes a publish message being received over a subscription connection
type Pub distr.Pub

// PresenceFunc is used to provide a presence string and its signature for the
// otter client. First returned string is the presence string, second is the
// signature.
type PresenceFunc func() (string, string, error)

// BackendPresence creates a PresenceFunc which can be used by a Client for a
// backend application
func BackendPresence(secret string) PresenceFunc {
	return func() (string, string, error) {
		p := "backend"
		sig := (auth.Auth{Key: secret}).Sign(p)
		return p, sig, nil
	}
}

func (c Client) randURL(scheme string, subs ...string) (string, error) {
	if c.HTTPS {
		scheme += "s"
	}
	addr := c.Addrs[rand.Intn(len(c.Addrs))]
	addr = srvclient.MaybeSRV(addr)
	var presence, sig string
	var err error
	if c.PresenceFunc != nil {
		if presence, sig, err = c.PresenceFunc(); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%s://%s/subs/%s?presence=%s&sig=%s", scheme, addr, strings.Join(subs, ","), presence, sig), nil
}

// Subscribe is used to create a single otter connection which will listen for
// incoming publishes from the given set of subscriptions. The publishes will be
// pushed to the given channel.
//
// The returned error channel is buffered by 1, and will have an error written
// to it if one is encountered, at which point this call is done.
//
// If stopCh is not nil, it can be close()'d by an separate go-routine to close
// the subsription connection. The returned error channel will be closed in this
// case.
func (c Client) Subscribe(pubCh chan<- Pub, stopCh chan struct{}, subs ...string) <-chan error {
	errCh := make(chan error, 1)

	u, err := c.randURL("ws", subs...)
	if err != nil {
		errCh <- err
		return errCh
	}

	conn, err := websocket.Dial(u, "", u)
	if err != nil {
		errCh <- err
		return errCh
	}

	innerStopCh := make(chan struct{})
	go func() {
		select {
		case <-stopCh:
		case <-innerStopCh:
		}
		conn.Close()
	}()

	go func() {
		defer close(innerStopCh)
		defer close(errCh)

		var p Pub
		for {
			err = websocket.JSON.Receive(conn, &p)
			if err != nil {
				errCh <- err
				return
			}
			pubCh <- p
		}
	}()

	return errCh
}

// Publish will publish the given message to all the subs
func (c Client) Publish(msg interface{}, subs ...string) error {
	u, err := c.randURL("http", subs...)
	if err != nil {
		return err
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	r, err := http.NewRequest("POST", u, bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	_, err = http.DefaultClient.Do(r)
	return err
}
