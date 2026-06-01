package main

import (
	"testing"
	"time"

	"nasnotify-go/internal/app"
)

func TestDurationFromMinutesUsesFallback(t *testing.T) {
	got := app.DurationFromMinutesForTest(0, 60)
	if got != time.Hour {
		t.Fatalf("DurationFromMinutesForTest(0, 60) = %v; want %v", got, time.Hour)
	}
}

func TestDurationFromMinutesSupportsFractionalMinutes(t *testing.T) {
	got := app.DurationFromMinutesForTest(0.5, 60)
	want := 30 * time.Second
	if got != want {
		t.Fatalf("DurationFromMinutesForTest(0.5, 60) = %v; want %v", got, want)
	}
}

func TestNormalizeClawBotCommand(t *testing.T) {
	tests := map[string]string{
		" QueryDeck! ":      "querydeck",
		"`control deck`":    "control deck",
		"control\u3000deck": "control deck",
		"CPU1":              "cpu1",
	}

	for input, want := range tests {
		if got := app.NormalizeClawBotCommandForTest(input); got != want {
			t.Fatalf("NormalizeClawBotCommandForTest(%q) = %q; want %q", input, got, want)
		}
	}
}
