package evasion

import (
	"math/rand"
	"sync"
)

var uaOnce sync.Once

func initRNG() {
	uaOnce.Do(func() {})
}

// RandUA returns a randomized browser user-agent string.
func RandUA() string {
	initRNG()
	return userAgents[rand.Intn(len(userAgents))]
}