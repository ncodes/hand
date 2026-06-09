package pairing

import (
	"fmt"
	"strings"
)

func ChallengeMessage(challenge Challenge) string {
	code := strings.TrimSpace(challenge.Code)
	source := strings.TrimSpace(challenge.Request.Source)
	if source == "" {
		source = "gateway"
	}

	return fmt.Sprintf(
		"Pair this %s chat with Hand by running:\n\n```shell\nhand gateway pairing approve %s %s\n```\n\nThis code expires soon.",
		source,
		source,
		code,
	)
}
