package distr

import (
	"encoding/json"
	. "testing"
	"time"

	"github.com/levenlabs/golib/testutil"
	"github.com/levenlabs/otter/conn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublish(t *T) {

	m := json.RawMessage(`{"foo":"blah","bar":"blahblah"}`)
	p := Pub{
		Conn:    conn.New(),
		Channel: testutil.RandStr(),
		Message: &m,
	}

	require.Nil(t, Publish(p))
	select {
	case p2 := <-PubCh:
		assert.Equal(t, p, p2)
	case <-time.After(1 * time.Second):
		t.Fatalf("timedout out waiting for publish")
	}
}
