package notify

import (
	"testing"

	"nasnotify-go/internal/notifycard"
)

func TestEnrichNotifyCardDefaultsToUGreenCloudApp(t *testing.T) {
	card := enrichNotifyCard(notifycard.Card{})
	if card.ActionURL != ugreenCloudAppDeepLink {
		t.Fatalf("ActionURL = %q; want %q", card.ActionURL, ugreenCloudAppDeepLink)
	}
}

func TestEnrichNotifyCardKeepsExplicitActionURL(t *testing.T) {
	const explicitURL = "https://nas.example.com/ugreen"
	card := enrichNotifyCard(notifycard.Card{ActionURL: explicitURL})
	if card.ActionURL != explicitURL {
		t.Fatalf("ActionURL = %q; want explicit URL %q", card.ActionURL, explicitURL)
	}
}
