package nas

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWechatProgressBar(t *testing.T) {
	emptyUnit := wechatProgressBar(0, 1)
	filledUnit := wechatProgressBar(100, 1)

	tests := []struct {
		name    string
		percent float64
		width   int
		want    string
	}{
		{name: "empty", percent: 0, width: 5, want: strings.Repeat(emptyUnit, 5)},
		{name: "half", percent: 50, width: 6, want: strings.Repeat(filledUnit, 3) + strings.Repeat(emptyUnit, 3)},
		{name: "full", percent: 100, width: 4, want: strings.Repeat(filledUnit, 4)},
		{name: "clamped", percent: 180, width: 4, want: strings.Repeat(filledUnit, 4)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wechatProgressBar(tt.percent, tt.width); got != tt.want {
				t.Fatalf("wechatProgressBar() = %q, want %q", got, tt.want)
			}
			if gotRunes := utf8.RuneCountInString(wechatProgressBar(tt.percent, tt.width)); gotRunes != tt.width {
				t.Fatalf("wechatProgressBar() rune count = %d, want %d", gotRunes, tt.width)
			}
		})
	}
}

func TestFormatBytesHuman(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{size: 0, want: "0 B"},
		{size: 512, want: "512 B"},
		{size: 1024 * 1024 * 5, want: "5.00 MB"},
		{size: 1024 * 1024 * 1024 * 12, want: "12.0 GB"},
	}

	for _, tt := range tests {
		if got := formatBytesHuman(tt.size); got != tt.want {
			t.Fatalf("formatBytesHuman(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}

func TestBuildUGreenSystemStatusPushContentUsesCharts(t *testing.T) {
	content := buildUGreenSystemStatusPushContent(&UGreenSystemInfo{
		UsageCpu:        25,
		CpuTemp:         45,
		UsageMemory:     50,
		MemoryUsed:      4 * 1024 * 1024 * 1024,
		MemoryTotal:     8 * 1024 * 1024 * 1024,
		NetworkReceive:  "1.2MB/s",
		NetworkTransmit: "88.0KB/s",
	}, "本机绿联 NAS")

	filledUnit := wechatProgressBar(100, 1)
	for _, want := range []string{"CPU", filledUnit, "内存", "网络速率"} {
		if !strings.Contains(content, want) {
			t.Fatalf("content does not contain %q:\n%s", want, content)
		}
	}
}

func TestParseUGreenPerfCommand(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		wantAction string
		wantMode   string
		wantOK     bool
	}{
		{
			name:       "compact fan command",
			command:    "风扇2",
			wantAction: "FAN",
			wantMode:   "2",
			wantOK:     true,
		},
		{
			name:       "spaced fan command",
			command:    "风扇 2",
			wantAction: "FAN",
			wantMode:   "2",
			wantOK:     true,
		},
		{
			name:       "compact cpu command",
			command:    "CPU1",
			wantAction: "CPU",
			wantMode:   "1",
			wantOK:     true,
		},
		{
			name:       "lowercase spaced cpu command",
			command:    "cpu 2",
			wantAction: "CPU",
			wantMode:   "2",
			wantOK:     true,
		},
		{
			name:       "english fan alias",
			command:    "fan3",
			wantAction: "FAN",
			wantMode:   "3",
			wantOK:     true,
		},
		{
			name:    "missing fan mode",
			command: "风扇",
			wantOK:  false,
		},
		{
			name:    "fan without numeric mode",
			command: "fan deck",
			wantOK:  false,
		},
		{
			name:    "cpu without numeric mode",
			command: "cpu deck",
			wantOK:  false,
		},
		{
			name:    "control deck is not a perf command",
			command: "control deck",
			wantOK:  false,
		},
		{
			name:    "unsupported command",
			command: "abc",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, mode, ok := parseUGreenPerfCommand(tt.command)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if action != tt.wantAction || mode != tt.wantMode {
				t.Fatalf("parseUGreenPerfCommand(%q) = (%q, %q), want (%q, %q)",
					tt.command, action, mode, tt.wantAction, tt.wantMode)
			}
		})
	}
}

func TestBuildUGreenPerfRequests(t *testing.T) {
	tests := []struct {
		name            string
		action          string
		mode            int
		wantRequests    []ugreenPerfRequest
		wantMessagePart string
		wantErr         bool
	}{
		{
			name:   "fan uses hardware endpoint first",
			action: "FAN",
			mode:   2,
			wantRequests: []ugreenPerfRequest{
				{Method: "GET", Path: "/ugreen/v1/hardware/fan/start", Params: map[string]string{"mode": "2"}},
				{Method: "POST", Path: "/ugreen/v1/taskmgr/power/fan", Params: map[string]string{"mode": "2"}},
			},
			wantMessagePart: "风扇切换为标准模式",
		},
		{
			name:   "cpu uses hardware endpoint first",
			action: "CPU",
			mode:   1,
			wantRequests: []ugreenPerfRequest{
				{Method: "POST", Path: "/ugreen/v1/hardware/cpu/frequency", Body: map[string]interface{}{"frequency": 1}},
				{Method: "POST", Path: "/ugreen/v1/taskmgr/power/cpu", Params: map[string]string{"mode": "1"}},
			},
			wantMessagePart: "CPU 切换为均衡模式",
		},
		{
			name:    "invalid fan mode rejected",
			action:  "FAN",
			mode:    9,
			wantErr: true,
		},
		{
			name:    "invalid cpu mode rejected",
			action:  "CPU",
			mode:    9,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests, message, err := buildUGreenPerfRequests(tt.action, tt.mode, "device")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(requests, tt.wantRequests) {
				t.Fatalf("requests = %#v, want %#v", requests, tt.wantRequests)
			}
			if !strings.Contains(message, tt.wantMessagePart) {
				t.Fatalf("message = %q, want to contain %q", message, tt.wantMessagePart)
			}
		})
	}
}
