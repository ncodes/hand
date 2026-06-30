package pairing

import (
	"fmt"

	"github.com/wandxy/morph/pkg/stringx"
)

func ChallengeMessage(challenge Challenge) string {
	code := stringx.String(challenge.Code).Trim()
	source := stringx.String(challenge.Request.Source).Trim()
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
