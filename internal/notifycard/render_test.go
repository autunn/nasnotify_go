package notifycard

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
)

func TestRenderPNG(t *testing.T) {
	card := Card{
		Title:     "系统状态概览",
		Device:    "书房绿联 NAS",
		Timestamp: "2026-06-01 15:04",
		Summary:   "当前系统运行正常，适合用来承载微信通知、固定查询和控制指令。",
		Badges:    []string{"网关在线", "绑定完成", "轮询 5 分钟"},
		Metrics: []Metric{
			{Label: "CPU 占用", Value: "25%", Hint: "温度 45°C", Tone: ToneGood, Chart: &MetricChart{Percent: 25}},
			{Label: "内存占用", Value: "50%", Hint: "4.0 / 8.0 GB", Tone: ToneWarm},
		},
		Sections: []Section{
			{Title: "设备信息", Lines: []string{"版本：UGOS Pro 1.0", "最近启动：2026-06-01 10:20"}},
			{Title: "存储概览", Lines: []string{"系统卷 · 72% · 1.4 TB / 2.0 TB", "媒体卷 · 18% · 200 GB / 1.0 TB"}},
		},
		Footer:    "图片发送失败时，会自动退回到原有的文字通知。",
		ActionURL: "https://nas.autunn.top/ugreen",
	}

	raw, err := RenderPNG(card)
	if err != nil {
		t.Fatalf("RenderPNG() error = %v", err)
	}
	if len(raw) < 1024 {
		t.Fatalf("RenderPNG() produced too few bytes: %d", len(raw))
	}

	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("png.Decode() error = %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != canvasWidth {
		t.Fatalf("image width = %d, want %d", bounds.Dx(), canvasWidth)
	}
	if bounds.Dy() < 900 {
		t.Fatalf("image height = %d, want >= 900", bounds.Dy())
	}

	topLeft := color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA)
	if topLeft.A != 255 {
		t.Fatalf("top-left pixel alpha = %d, want 255", topLeft.A)
	}
	if topLeft.R != 0 || topLeft.G != 0 || topLeft.B != 0 {
		t.Fatalf("top-left pixel = %#v, want pure black", topLeft)
	}
}

func TestBuildActionPanel(t *testing.T) {
	panel := buildActionPanel("https://nas.autunn.top/ugreen/v1/desktop")
	if panel == nil {
		t.Fatal("buildActionPanel() returned nil")
	}
	if panel.QRImage == nil {
		t.Fatal("buildActionPanel() did not create QR image")
	}
	if got := panel.DisplayURL; got != "nas.autunn.top/ugreen/v1/desktop" {
		t.Fatalf("DisplayURL = %q, want %q", got, "nas.autunn.top/ugreen/v1/desktop")
	}
}

func TestBuildActionPanelForUGreenCloudApp(t *testing.T) {
	panel := buildActionPanel("ugreenpro://h5.ugnas.com")
	if panel == nil {
		t.Fatal("buildActionPanel() returned nil")
	}
	if panel.QRImage == nil {
		t.Fatal("buildActionPanel() did not create QR image")
	}
	if got := panel.DisplayURL; got != "绿联云 App" {
		t.Fatalf("DisplayURL = %q, want %q", got, "绿联云 App")
	}
	if got := actionPanelTitle(*panel); got != "扫码打开绿联云" {
		t.Fatalf("actionPanelTitle() = %q, want %q", got, "扫码打开绿联云")
	}
	if !isUGreenCloudDeepLink(panel.URL) {
		t.Fatalf("isUGreenCloudDeepLink(%q) = false; want true", panel.URL)
	}
}
