package notify

import (
	"strings"

	"nasnotify-go/internal/config"
	"nasnotify-go/internal/notifycard"
)

func enrichNotifyCard(card notifycard.Card) notifycard.Card {
	if strings.TrimSpace(card.ActionURL) != "" {
		return card
	}

	snapshot := config.GetConfigSnapshot()
	if actionURL := strings.TrimSpace(snapshot.NasURL); actionURL != "" {
		card.ActionURL = actionURL
	}
	return card
}
