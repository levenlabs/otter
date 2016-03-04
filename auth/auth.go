// Package auth deals with creating and verifying signatures for strings
package auth

// TODO I've written something like this code like 5 times now, we should really
// put this in golib or something

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/levenlabs/golib/timeutil"
)

// Auth handles creating and verifying tokens given a specific set of parameters
type Auth struct {
	Key     string
	Timeout time.Duration
}

const sep = "_"

func (a Auth) makeMac(val string, t []byte) []byte {
	mac := hmac.New(sha256.New, []byte(a.Key))
	mac.Write(t)
	mac.Write([]byte(sep))
	mac.Write([]byte(val))
	return mac.Sum(nil)
}

// Sign takes a value and returns a signature for that value
func (a Auth) Sign(val string) string {
	now := timeutil.TimestampNow()
	nows := now.String()

	return hex.EncodeToString(a.makeMac(val, []byte(nows))) + sep + nows
}

// Verify takes a signature and a value it supposedly signs and verifies that
// that is the case. If Timeout is not zero then this will ensure the signature
// has not timed out as well.
func (a Auth) Verify(sig, val string) bool {
	p := strings.SplitN(sig, "_", 2)
	if len(p) != 2 {
		return false
	}
	macStr, tStr := p[0], p[1]
	mac, err := hex.DecodeString(macStr)
	if err != nil {
		return false
	}

	if !hmac.Equal(a.makeMac(val, []byte(tStr)), mac) {
		return false
	}
	if a.Timeout == 0 {
		return true
	}

	tf, err := strconv.ParseFloat(tStr, 64)
	if err != nil {
		return false
	}

	t := timeutil.TimestampFromFloat64(tf)
	if time.Since(t.Time) > a.Timeout {
		return false
	}
	return true
}
