package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func newRunID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return fmt.Sprintf("run-%d-%s", time.Now().Unix(), hex.EncodeToString(b[:]))
	}
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}