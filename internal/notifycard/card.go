package notifycard

import "time"

type Card struct {
	Title     string
	Device    string
	Timestamp string
	Summary   string
	Badges    []string
	Metrics   []Metric
	Sections  []Section
	Footer    string
	ActionURL string
}

type Metric struct {
	Label string
	Value string
	Hint  string
	Tone  Tone
	Chart *MetricChart
}

type Section struct {
	Title string
	Lines []string
}

type MetricChart struct {
	Percent float64
}

type Tone string

const (
	ToneDefault Tone = "default"
	ToneGood    Tone = "good"
	ToneWarm    Tone = "warm"
	ToneDanger  Tone = "danger"
)

func (c Card) WithTimestamp(now time.Time) Card {
	if c.Timestamp == "" {
		c.Timestamp = now.Format("2006-01-02 15:04")
	}
	return c
}
