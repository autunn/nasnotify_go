package notify

import (
	"strings"

	"nasnotify-go/internal/notifycard"
)

const ugreenCloudAppDeepLink = "https://a.app.qq.com/o/simple.jsp?pkgname=com.ugreen.pro"

func enrichNotifyCard(card notifycard.Card) notifycard.Card {
	if strings.TrimSpace(card.ActionURL) != "" {
		return card
	}

	card.ActionURL = ugreenCloudAppDeepLink
	return card
}
