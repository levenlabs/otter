package auth

import (
	. "testing"
	"time"

	"github.com/levenlabs/golib/testutil"
	"github.com/stretchr/testify/assert"
)

func TestAuth(t *T) {
	a := Auth{
		Key: testutil.RandStr(),
	}

	val := testutil.RandStr()
	sig := a.Sign(val)
	t.Logf("val:%q sig:%q", val, sig)
	assert.True(t, a.Verify(sig, val))

	a.Timeout = 100 * time.Millisecond
	assert.True(t, a.Verify(sig, val))

	time.Sleep(100 * time.Millisecond)
	assert.False(t, a.Verify(sig, val))

	a.Timeout = 1 * time.Second
	assert.True(t, a.Verify(sig, val))
}
