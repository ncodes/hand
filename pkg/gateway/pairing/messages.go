package pairing

import (
	"fmt"

	"github.com/wandxy/morph/pkg/str"
)

func ChallengeMessage(challenge Challenge) string {
	stringValue1 := str.String(challenge.Code)
	code := stringValue1.Trim()
	stringValue2 := str.String(challenge.Request.Source)
	source := stringValue2.Trim()
	if source == "" {
		source = "gateway"
	}

	return fmt.Sprintf(
		"Pair this %s chat with Morph by running:\n\n```shell\nmorph gateway pairing approve %s %s\n```\n\nThis code expires soon.",
		source,
		source,
		code,
	)
}
