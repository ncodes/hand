package trace

import (
	"crypto/rand"
	"encoding/hex"
)

var readRandom = rand.Read

func randomSuffix() string {
	bytes := make([]byte, 4)
	if _, err := readRandom(bytes); err != nil {
		return "trace"
	}

	return hex.EncodeToString(bytes)
}
