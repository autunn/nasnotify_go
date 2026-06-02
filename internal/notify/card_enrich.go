package notify

import (
	"strings"

	"nasnotify-go/internal/notifycard"
)

const ugreenCloudAppDeepLink = "ugreenpro://h5.ugnas.com"

func enrichNotifyCard(card notifycard.Card) notifycard.Card {
	if strings.TrimSpace(card.ActionURL) != "" {
		return card
	}

	card.ActionURL = ugreenCloudAppDeepLink
	return card
}
