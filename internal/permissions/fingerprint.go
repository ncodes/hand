package permissions

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func Fingerprint(authorization AuthorizationContext, operation Operation) string {
	values := []string{
		string(authorization.Actor.Kind),
		authorization.Actor.ID,
		authorization.Profile,
		string(authorization.SurfaceKind),
		string(authorization.Surface),
		operation.Tool,
		string(operation.Resource),
		string(operation.Action),
		strings.Join(effectsToStrings(operation.Effects), ","),
		operation.Target,
		operation.OwnerID,
	}
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return hex.EncodeToString(sum[:])
}

func effectsToStrings(effects []Effect) []string {
	values := make([]string, len(effects))
	for index, effect := range effects {
		values[index] = string(effect)
	}
	return values
}
