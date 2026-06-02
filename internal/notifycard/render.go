package notifycard

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fogleman/gg"
	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

//go:embed assets/Dengb.ttf
var embeddedTitleFont []byte

//go:embed assets/Deng.ttf
var embeddedBodyFont []byte

const (
	canvasWidth       = 1280
	pagePadding       = 24.0
	outerRadius       = 18.0
	blockRadius       = 14.0
	metricRadius      = 14.0
	contentPadding    = 56.0
	sectionGap        = 20.0
	metricHeight      = 236.0
	minCanvasHeight   = 980
	headerBottomGap   = 24.0
	sectionLineGap    = 10.0
	badgeHeight       = 42.0
	badgeGap          = 10.0
	actionPanelHeight = 248.0
	actionQRWrapSize  = 192.0
	actionPanelInset  = 28.0
)

var (
	titleFontOnce   sync.Once
	titleFontParsed *opentype.Font
	titleFontErr    error

	bodyFontOnce   sync.Once
	bodyFontParsed *opentype.Font
	bodyFontErr    error
)

type theme struct {
	background   color.Color
	cardTop      color.Color
	cardBottom   color.Color
	cardStroke   color.Color
	topLine      color.Color
	divider      color.Color
	text         color.Color
	textSoft     color.Color
	textMute     color.Color
	title        color.Color
	accent       color.Color
	chipBg       color.Color
	chipStroke   color.Color
	chipText     color.Color
	panelTop     color.Color
	panelBottom  color.Color
	panelStroke  color.Color
	barTrack     color.Color
	good         color.Color
	warm         color.Color
	danger       color.Color
	actionHint   color.Color
	qrWrapBg     color.Color
	qrWrapStroke color.Color
	qrCodeBg     color.Color
}

type badgeItem struct {
	Text  string
	Width float64
}

type actionPanel struct {
	URL        string
	DisplayURL string
	QRImage    image.Image
}

func RenderPNG(card Card) ([]byte, error) {
	card = card.WithTimestamp(time.Now())

	m := newTheme()
	measure := gg.NewContext(10, 10)
	action := buildActionPanel(strings.TrimSpace(card.ActionURL))

	brandFace, err := titleFontFace(21)
	if err != nil {
		return nil, err
	}
	titleFace, _, err := fitTitleFace(measure, strings.TrimSpace(card.Title), float64(canvasWidth)-pagePadding*2-contentPadding*2-260, 68, 48)
	if err != nil {
		return nil, err
	}
	metaFace, err := titleFontFace(24)
	if err != nil {
		return nil, err
	}
	summaryFace, err := bodyFontFace(30)
	if err != nil {
		return nil, err
	}
	badgeFace, err := titleFontFace(20)
	if err != nil {
		return nil, err
	}
	metricLabelFace, err := titleFontFace(24)
	if err != nil {
		return nil, err
	}
	metricHintFace, err := titleFontFace(20)
	if err != nil {
		return nil, err
	}
	sectionTitleFace, err := titleFontFace(30)
	if err != nil {
		return nil, err
	}
	sectionBodyFace, err := bodyFontFace(27)
	if err != nil {
		return nil, err
	}
	actionTitleFace, err := titleFontFace(28)
	if err != nil {
		return nil, err
	}
	actionHintFace, err := bodyFontFace(24)
	if err != nil {
		return nil, err
	}

	outerWidth := float64(canvasWidth) - pagePadding*2
	innerWidth := outerWidth - contentPadding*2

	headerHeight := measureHeaderHeight(measure, card, innerWidth, brandFace, titleFace, metaFace, summaryFace, badgeFace)
	metricsHeight := measureMetricsHeight(card.Metrics)
	sectionsHeight := measureSectionsHeight(measure, card.Sections, innerWidth, sectionTitleFace, sectionBodyFace)
	footerHeight := measureFooterHeight(measure, card.Footer, innerWidth, sectionTitleFace, sectionBodyFace)
	actionHeight := measureActionPanelHeight(action)

	contentHeight := metricsHeight
	if metricsHeight > 0 && sectionsHeight > 0 {
		contentHeight += sectionGap
	}
	contentHeight += sectionsHeight
	if footerHeight > 0 {
		if contentHeight > 0 {
			contentHeight += sectionGap
		}
		contentHeight += footerHeight
	}
	if actionHeight > 0 {
		if contentHeight > 0 {
			contentHeight += sectionGap
		}
		contentHeight += actionHeight
	}

	totalHeight := int(math.Ceil(pagePadding*2 + headerHeight + headerBottomGap + contentHeight + contentPadding))
	if totalHeight < minCanvasHeight {
		totalHeight = minCanvasHeight
	}

	dc := gg.NewContext(canvasWidth, totalHeight)
	drawCanvasBackground(dc, float64(totalHeight), m)

	outerX := pagePadding
	outerY := pagePadding
	outerHeight := float64(totalHeight) - pagePadding*2

	drawOuterCard(dc, outerX, outerY, outerWidth, outerHeight, m)
	contentStartY := drawHeader(dc, outerX, outerY, outerWidth, card, m, innerWidth, brandFace, titleFace, metaFace, summaryFace, badgeFace)
	drawDivider(dc, outerX+contentPadding, contentStartY-10, innerWidth, m.divider)
	drawContent(dc, outerX, contentStartY+headerBottomGap, outerWidth, card, action, m, metricLabelFace, metricHintFace, sectionTitleFace, sectionBodyFace, actionTitleFace, actionHintFace)

	var out bytes.Buffer
	if err := png.Encode(&out, dc.Image()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func measureHeaderHeight(dc *gg.Context, card Card, innerWidth float64, brandFace, titleFace, metaFace, summaryFace, badgeFace font.Face) float64 {
	cursorY := 36.0
	cursorY += lineHeight(brandFace)
	cursorY += 16
	cursorY += wrappedHeight(dc, titleFace, card.Title, innerWidth, 1.08)
	cursorY += 14
	if strings.TrimSpace(card.Device) != "" {
		cursorY += lineHeight(metaFace)
		cursorY += 12
	}
	if len(compactBadges(card.Badges)) > 0 {
		cursorY += badgesHeight(dc, badgeFace, card.Badges, innerWidth)
		cursorY += 16
	}
	if strings.TrimSpace(card.Summary) != "" {
		cursorY += wrappedHeight(dc, summaryFace, card.Summary, innerWidth, 1.24)
		cursorY += 6
	}
	return cursorY + 18
}

func drawHeader(dc *gg.Context, x, y, width float64, card Card, m theme, innerWidth float64, brandFace, titleFace, metaFace, summaryFace, badgeFace font.Face) float64 {
	left := x + contentPadding
	right := x + width - contentPadding
	cursorY := y + 36

	dc.SetFontFace(brandFace)
	dc.SetColor(m.textMute)
	dc.DrawString("NAS 通知中心", left, cursorY+lineHeight(brandFace)-4)

	cursorY += lineHeight(brandFace) + 16

	dc.SetFontFace(titleFace)
	dc.SetColor(m.title)
	titleHeight := wrappedHeight(dc, titleFace, card.Title, innerWidth, 1.08)
	dc.DrawStringWrapped(strings.TrimSpace(card.Title), left, cursorY, 0, 0, innerWidth, 1.08, gg.AlignLeft)
	cursorY += titleHeight + 14

	chipTop := y + 30
	drawTimestampChip(dc, right, chipTop, strings.TrimSpace(card.Timestamp), m, badgeFace)

	if device := strings.TrimSpace(card.Device); device != "" {
		dc.SetFontFace(metaFace)
		dc.SetColor(m.textSoft)
		dc.DrawString("设备 / "+device, left, cursorY+lineHeight(metaFace)-4)
		cursorY += lineHeight(metaFace) + 12
	}

	if badges := compactBadges(card.Badges); len(badges) > 0 {
		cursorY += drawBadges(dc, left, cursorY, innerWidth, badges, m, badgeFace)
		cursorY += 16
	}

	if summary := strings.TrimSpace(card.Summary); summary != "" {
		dc.SetFontFace(summaryFace)
		dc.SetColor(m.text)
		dc.DrawStringWrapped(summary, left, cursorY, 0, 0, innerWidth, 1.24, gg.AlignLeft)
		cursorY += wrappedHeight(dc, summaryFace, summary, innerWidth, 1.24) + 6
	}

	return cursorY + 18
}

func drawTimestampChip(dc *gg.Context, right, top float64, text string, m theme, face font.Face) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	dc.SetFontFace(face)
	textWidth, _ := dc.MeasureString(text)
	chipWidth := math.Max(textWidth+34, 184)
	chipX := right - chipWidth
	chipY := top

	dc.DrawRoundedRectangle(chipX, chipY, chipWidth, badgeHeight, badgeHeight/2)
	dc.SetColor(m.chipBg)
	dc.Fill()
	dc.DrawRoundedRectangle(chipX, chipY, chipWidth, badgeHeight, badgeHeight/2)
	dc.SetColor(m.chipStroke)
	dc.SetLineWidth(1.1)
	dc.Stroke()

	dc.SetColor(m.chipText)
	dc.DrawStringAnchored(text, chipX+chipWidth/2, chipY+badgeHeight/2+0.5, 0.5, 0.4)
}

func drawContent(dc *gg.Context, x, y, width float64, card Card, action *actionPanel, m theme, metricLabelFace, metricHintFace, sectionTitleFace, sectionBodyFace, actionTitleFace, actionHintFace font.Face) {
	cursorY := y
	innerWidth := width - contentPadding*2
	left := x + contentPadding

	if len(card.Metrics) > 0 {
		columnWidth := (innerWidth - sectionGap) / 2
		for index, metric := range card.Metrics {
			col := float64(index % 2)
			row := float64(index / 2)
			metricX := left + col*(columnWidth+sectionGap)
			metricY := cursorY + row*(metricHeight+sectionGap)
			drawMetric(dc, metricX, metricY, columnWidth, metricHeight, metric, m, metricLabelFace, metricHintFace)
		}
		cursorY += measureMetricsHeight(card.Metrics)
		if len(card.Sections) > 0 || strings.TrimSpace(card.Footer) != "" || action != nil {
			cursorY += sectionGap
		}
	}

	for index, section := range card.Sections {
		height := calcSectionHeight(dc, section, innerWidth, sectionTitleFace, sectionBodyFace)
		drawSection(dc, left, cursorY, innerWidth, height, section, m, sectionTitleFace, sectionBodyFace)
		cursorY += height
		if index < len(card.Sections)-1 || strings.TrimSpace(card.Footer) != "" || action != nil {
			cursorY += sectionGap
		}
	}

	if footer := strings.TrimSpace(card.Footer); footer != "" {
		footerSection := Section{
			Title: "补充说明",
			Lines: []string{footer},
		}
		height := calcSectionHeight(dc, footerSection, innerWidth, sectionTitleFace, sectionBodyFace)
		drawSection(dc, left, cursorY, innerWidth, height, footerSection, m, sectionTitleFace, sectionBodyFace)
		cursorY += height
		if action != nil {
			cursorY += sectionGap
		}
	}

	if action != nil {
		drawActionPanel(dc, left, cursorY, innerWidth, *action, m, actionTitleFace, actionHintFace)
	}
}

func drawMetric(dc *gg.Context, x, y, width, height float64, metric Metric, m theme, labelFace, hintFace font.Face) {
	fill := gg.NewLinearGradient(x, y, x, y+height)
	fill.AddColorStop(0, m.panelTop)
	fill.AddColorStop(1, m.panelBottom)

	dc.DrawRoundedRectangle(x, y, width, height, metricRadius)
	dc.SetFillStyle(fill)
	dc.Fill()

	dc.DrawRoundedRectangle(x, y, width, height, metricRadius)
	dc.SetColor(m.panelStroke)
	dc.SetLineWidth(1.0)
	dc.Stroke()

	dc.SetFontFace(labelFace)
	dc.SetColor(m.textSoft)
	labelY := y + 50
	dc.DrawString(strings.TrimSpace(metric.Label), x+24, labelY)

	valueTop := y + 80
	if metric.Chart != nil {
		drawMetricChart(dc, x+24, y+70, width-48, 18, metric.Chart.Percent, toneColor(metric.Tone, m), m.barTrack)
		valueTop = y + 98
	}

	valueFace, _, err := fitFontFace(titleFontFace, dc, strings.TrimSpace(metric.Value), width-48, 60, 36)
	if err == nil {
		dc.SetFontFace(valueFace)
		dc.SetColor(toneColor(metric.Tone, m))
		valueText := strings.TrimSpace(metric.Value)
		valueHeight := wrappedHeight(dc, valueFace, valueText, width-48, 1.06)
		dc.DrawStringWrapped(valueText, x+24, valueTop, 0, 0, width-48, 1.06, gg.AlignLeft)

		if hint := strings.TrimSpace(metric.Hint); hint != "" {
			hintTop := valueTop + valueHeight + 10
			dc.SetFontFace(hintFace)
			dc.SetColor(m.textSoft)
			dc.DrawStringWrapped(hint, x+24, hintTop, 0, 0, width-48, 1.15, gg.AlignLeft)
		}
	}
}

func drawMetricChart(dc *gg.Context, x, y, width, height, percent float64, fillColor, trackColor color.Color) {
	percent = clampMetricPercent(percent)
	radius := height / 2

	dc.DrawRoundedRectangle(x, y, width, height, radius)
	dc.SetColor(trackColor)
	dc.Fill()

	if percent <= 0 {
		return
	}

	fillWidth := width * percent / 100
	if fillWidth < height*0.9 {
		fillWidth = height * 0.9
	}
	if fillWidth > width {
		fillWidth = width
	}

	dc.DrawRoundedRectangle(x, y, fillWidth, height, radius)
	dc.SetColor(fillColor)
	dc.Fill()

}

func drawSection(dc *gg.Context, x, y, width, height float64, section Section, m theme, titleFace, bodyFace font.Face) {
	fill := gg.NewLinearGradient(x, y, x, y+height)
	fill.AddColorStop(0, m.panelTop)
	fill.AddColorStop(1, m.panelBottom)

	dc.DrawRoundedRectangle(x, y, width, height, blockRadius)
	dc.SetFillStyle(fill)
	dc.Fill()

	dc.DrawRoundedRectangle(x, y, width, height, blockRadius)
	dc.SetColor(m.panelStroke)
	dc.SetLineWidth(1.0)
	dc.Stroke()

	dc.SetFontFace(titleFace)
	dc.SetColor(m.title)
	dc.DrawString(strings.TrimSpace(section.Title), x+28, y+42)

	drawDivider(dc, x+28, y+62, width-56, m.divider)

	dc.SetFontFace(bodyFace)
	bodyLineHeight := lineHeight(bodyFace) * 1.18
	cursorY := y + 92
	for _, line := range section.Lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		wrapped := wrappedLines(dc, bodyFace, line, width-96)
		if len(wrapped) == 0 {
			continue
		}

		for index, item := range wrapped {
			if index == 0 {
				dc.DrawCircle(x+36, cursorY-10, 3.2)
				dc.SetColor(m.accent)
				dc.Fill()
			}
			dc.SetColor(m.text)
			dc.DrawString(item, x+56, cursorY)
			cursorY += bodyLineHeight
		}
		cursorY += sectionLineGap
	}
}

func measureMetricsHeight(metrics []Metric) float64 {
	if len(metrics) == 0 {
		return 0
	}
	rows := math.Ceil(float64(len(metrics)) / 2)
	return rows*metricHeight + math.Max(0, rows-1)*sectionGap
}

func measureSectionsHeight(dc *gg.Context, sections []Section, width float64, titleFace, bodyFace font.Face) float64 {
	total := 0.0
	for index, section := range sections {
		total += calcSectionHeight(dc, section, width, titleFace, bodyFace)
		if index < len(sections)-1 {
			total += sectionGap
		}
	}
	return total
}

func measureFooterHeight(dc *gg.Context, footer string, width float64, titleFace, bodyFace font.Face) float64 {
	footer = strings.TrimSpace(footer)
	if footer == "" {
		return 0
	}
	return calcSectionHeight(dc, Section{
		Title: "补充说明",
		Lines: []string{footer},
	}, width, titleFace, bodyFace)
}

func measureActionPanelHeight(action *actionPanel) float64 {
	if action == nil {
		return 0
	}
	return actionPanelHeight
}

func calcSectionHeight(dc *gg.Context, section Section, width float64, titleFace, bodyFace font.Face) float64 {
	bodyWidth := width - 96
	total := 92.0
	bodyLineHeight := lineHeight(bodyFace) * 1.18
	for _, line := range section.Lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines := wrappedLines(dc, bodyFace, line, bodyWidth)
		if len(lines) == 0 {
			continue
		}
		total += float64(len(lines))*bodyLineHeight + sectionLineGap
	}
	if total < 138 {
		total = 138
	}
	return total + 16
}

func drawCanvasBackground(dc *gg.Context, height float64, m theme) {
	dc.DrawRectangle(0, 0, float64(canvasWidth), height)
	dc.SetColor(m.background)
	dc.Fill()
}

func drawOuterCard(dc *gg.Context, x, y, width, height float64, m theme) {
	fill := gg.NewLinearGradient(x, y, x, y+height)
	fill.AddColorStop(0, m.cardTop)
	fill.AddColorStop(1, m.cardBottom)

	dc.DrawRoundedRectangle(x, y, width, height, outerRadius)
	dc.SetFillStyle(fill)
	dc.Fill()

	dc.DrawRoundedRectangle(x, y, width, height, outerRadius)
	dc.SetColor(m.cardStroke)
	dc.SetLineWidth(1.2)
	dc.Stroke()

	dc.DrawRoundedRectangle(x+contentPadding, y+30, width-contentPadding*2, 1.4, 0.7)
	dc.SetColor(m.topLine)
	dc.Fill()
}

func drawDivider(dc *gg.Context, x, y, width float64, c color.Color) {
	dc.DrawRoundedRectangle(x, y, width, 1.2, 0.6)
	dc.SetColor(c)
	dc.Fill()
}

func drawActionPanel(dc *gg.Context, x, y, width float64, action actionPanel, m theme, titleFace, hintFace font.Face) {
	fill := gg.NewLinearGradient(x, y, x, y+actionPanelHeight)
	fill.AddColorStop(0, m.panelTop)
	fill.AddColorStop(1, m.panelBottom)

	dc.DrawRoundedRectangle(x, y, width, actionPanelHeight, blockRadius)
	dc.SetFillStyle(fill)
	dc.Fill()

	dc.DrawRoundedRectangle(x, y, width, actionPanelHeight, blockRadius)
	dc.SetColor(m.panelStroke)
	dc.SetLineWidth(1.0)
	dc.Stroke()

	left := x + actionPanelInset
	top := y + actionPanelInset
	qrX := x + width - actionPanelInset - actionQRWrapSize
	qrY := y + (actionPanelHeight-actionQRWrapSize)/2
	textWidth := qrX - left - 28

	dc.SetFontFace(titleFace)
	dc.SetColor(m.title)
	dc.DrawString(actionPanelTitle(action), left, top+lineHeight(titleFace)-4)

	copyTop := top + lineHeight(titleFace) + 18
	dc.SetFontFace(hintFace)
	dc.SetColor(m.textSoft)
	dc.DrawStringWrapped(actionPanelHint(action), left, copyTop, 0, 0, textWidth, 1.22, gg.AlignLeft)

	labelTop := y + actionPanelHeight - actionPanelInset - 88
	dc.SetColor(m.actionHint)
	dc.DrawString(actionPanelLabel(action), left, labelTop)

	urlBoxTop := labelTop + 16
	urlBoxHeight := 64.0
	dc.DrawRoundedRectangle(left, urlBoxTop, textWidth, urlBoxHeight, 14)
	dc.SetColor(m.chipBg)
	dc.Fill()
	dc.DrawRoundedRectangle(left, urlBoxTop, textWidth, urlBoxHeight, 14)
	dc.SetColor(m.chipStroke)
	dc.SetLineWidth(1.0)
	dc.Stroke()

	display := strings.TrimSpace(action.DisplayURL)
	displayFace, _, err := fitBodyFace(dc, display, textWidth-28, 26, 18)
	if err == nil {
		dc.SetFontFace(displayFace)
	}
	dc.SetColor(m.chipText)
	dc.DrawStringWrapped(display, left+14, urlBoxTop+20, 0, 0, textWidth-28, 1.08, gg.AlignLeft)

	dc.DrawRoundedRectangle(qrX, qrY, actionQRWrapSize, actionQRWrapSize, 20)
	dc.SetColor(m.qrWrapBg)
	dc.Fill()
	dc.DrawRoundedRectangle(qrX, qrY, actionQRWrapSize, actionQRWrapSize, 20)
	dc.SetColor(m.qrWrapStroke)
	dc.SetLineWidth(1.1)
	dc.Stroke()

	if action.QRImage != nil {
		qrInset := 16.0
		qrInnerX := qrX + qrInset
		qrInnerY := qrY + qrInset
		qrInnerSize := actionQRWrapSize - qrInset*2
		dc.DrawRoundedRectangle(qrInnerX, qrInnerY, qrInnerSize, qrInnerSize, 12)
		dc.SetColor(m.qrCodeBg)
		dc.Fill()
		dc.DrawImageAnchored(action.QRImage, int(qrInnerX+qrInnerSize/2), int(qrInnerY+qrInnerSize/2), 0.5, 0.5)
	}
}

func buildActionPanel(rawURL string) *actionPanel {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}

	panel := &actionPanel{
		URL:        rawURL,
		DisplayURL: formatActionDisplayURL(rawURL),
	}

	qr, err := qrcode.New(rawURL, qrcode.Medium)
	if err != nil {
		return panel
	}
	panel.QRImage = qr.Image(int(actionQRWrapSize - 32))
	return panel
}

func actionPanelTitle(action actionPanel) string {
	if isUGreenCloudDeepLink(action.URL) {
		return "扫码打开绿联云"
	}
	return "扫码打开 NAS"
}

func actionPanelHint(action actionPanel) string {
	if isUGreenCloudDeepLink(action.URL) {
		return "右下角二维码可直接拉起手机绿联云 App，适合快速回到 NAS 管理入口。"
	}
	return "右下角二维码可直接跳转到 NAS 页面，适合在手机端快速返回控制台。"
}

func actionPanelLabel(action actionPanel) string {
	if isUGreenCloudDeepLink(action.URL) {
		return "跳转目标"
	}
	return "NAS 地址"
}

func formatActionDisplayURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return ellipsize(strings.TrimSpace(rawURL), 72)
	}
	if isUGreenCloudDeepLink(rawURL) {
		return "绿联云 App"
	}
	if parsed.Host == "" {
		return ellipsize(strings.TrimSpace(rawURL), 72)
	}

	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" || path == "/" {
		return parsed.Host
	}
	return ellipsize(parsed.Host+path, 72)
}

func isUGreenCloudDeepLink(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return false
	}
	if strings.EqualFold(parsed.Scheme, "ugreenpro") && strings.EqualFold(parsed.Host, "h5.ugnas.com") {
		return true
	}
	if isUGreenCloudAppQQLink(parsed) {
		return true
	}
	if !strings.EqualFold(parsed.Scheme, "intent") || !strings.EqualFold(parsed.Host, "h5.ugnas.com") {
		return false
	}
	return strings.EqualFold(intentFragmentValue(parsed.Fragment, "scheme"), "ugreenpro") &&
		strings.EqualFold(intentFragmentValue(parsed.Fragment, "package"), "com.ugreen.pro")
}

func intentFragmentValue(fragment, key string) string {
	if !strings.HasPrefix(fragment, "Intent;") {
		return ""
	}
	for _, part := range strings.Split(fragment, ";") {
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(name, key) {
			return value
		}
	}
	return ""
}

func isUGreenCloudAppQQLink(parsed *url.URL) bool {
	if parsed == nil {
		return false
	}
	if !strings.EqualFold(parsed.Scheme, "https") && !strings.EqualFold(parsed.Scheme, "http") {
		return false
	}
	if !strings.EqualFold(parsed.Host, "a.app.qq.com") || parsed.EscapedPath() != "/o/simple.jsp" {
		return false
	}
	return strings.EqualFold(parsed.Query().Get("pkgname"), "com.ugreen.pro")
}

func ellipsize(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func compactBadges(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func badgesHeight(dc *gg.Context, face font.Face, badges []string, maxWidth float64) float64 {
	rows := badgeRows(dc, face, badges, maxWidth)
	if len(rows) == 0 {
		return 0
	}
	return float64(len(rows))*badgeHeight + float64(len(rows)-1)*badgeGap
}

func drawBadges(dc *gg.Context, x, y, maxWidth float64, badges []string, m theme, face font.Face) float64 {
	rows := badgeRows(dc, face, badges, maxWidth)
	if len(rows) == 0 {
		return 0
	}

	dc.SetFontFace(face)
	cursorY := y
	for _, row := range rows {
		cursorX := x
		for _, item := range row {
			dc.DrawRoundedRectangle(cursorX, cursorY, item.Width, badgeHeight, badgeHeight/2)
			dc.SetColor(m.chipBg)
			dc.Fill()
			dc.DrawRoundedRectangle(cursorX, cursorY, item.Width, badgeHeight, badgeHeight/2)
			dc.SetColor(m.chipStroke)
			dc.SetLineWidth(1.0)
			dc.Stroke()
			dc.SetColor(m.chipText)
			dc.DrawStringAnchored(item.Text, cursorX+item.Width/2, cursorY+badgeHeight/2+0.5, 0.5, 0.4)
			cursorX += item.Width + badgeGap
		}
		cursorY += badgeHeight + badgeGap
	}
	return float64(len(rows))*badgeHeight + float64(len(rows)-1)*badgeGap
}

func badgeRows(dc *gg.Context, face font.Face, badges []string, maxWidth float64) [][]badgeItem {
	badges = compactBadges(badges)
	if len(badges) == 0 {
		return nil
	}
	dc.SetFontFace(face)

	rows := make([][]badgeItem, 0, 2)
	row := make([]badgeItem, 0, len(badges))
	rowWidth := 0.0

	for _, badge := range badges {
		textWidth, _ := dc.MeasureString(badge)
		item := badgeItem{
			Text:  badge,
			Width: textWidth + 32,
		}
		nextWidth := item.Width
		if len(row) > 0 {
			nextWidth += badgeGap
		}
		if rowWidth+nextWidth > maxWidth && len(row) > 0 {
			rows = append(rows, row)
			row = []badgeItem{item}
			rowWidth = item.Width
			continue
		}
		row = append(row, item)
		rowWidth += nextWidth
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

func wrappedHeight(dc *gg.Context, face font.Face, text string, width, spacing float64) float64 {
	lines := wrappedLines(dc, face, text, width)
	if len(lines) == 0 {
		return 0
	}
	return float64(len(lines)) * lineHeight(face) * spacing
}

func wrappedLines(dc *gg.Context, face font.Face, text string, width float64) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	dc.SetFontFace(face)
	lines := dc.WordWrap(text, width)
	if len(lines) == 0 {
		return []string{text}
	}
	return lines
}

func lineHeight(face font.Face) float64 {
	metrics := face.Metrics()
	return float64((metrics.Ascent + metrics.Descent).Ceil())
}

func fitBodyFace(dc *gg.Context, text string, maxWidth, startSize, minSize float64) (font.Face, float64, error) {
	return fitFontFace(bodyFontFace, dc, text, maxWidth, startSize, minSize)
}

func fitTitleFace(dc *gg.Context, text string, maxWidth, startSize, minSize float64) (font.Face, float64, error) {
	return fitFontFace(titleFontFace, dc, text, maxWidth, startSize, minSize)
}

func fitFontFace(faceFunc func(float64) (font.Face, error), dc *gg.Context, text string, maxWidth, startSize, minSize float64) (font.Face, float64, error) {
	for size := startSize; size >= minSize; size -= 2 {
		face, err := faceFunc(size)
		if err != nil {
			return nil, 0, err
		}
		dc.SetFontFace(face)
		width, _ := dc.MeasureString(text)
		if width <= maxWidth {
			return face, size, nil
		}
	}
	face, err := faceFunc(minSize)
	return face, minSize, err
}

func titleFontFace(size float64) (font.Face, error) {
	titleFontOnce.Do(func() {
		titleFontParsed, titleFontErr = opentype.Parse(embeddedTitleFont)
	})
	if titleFontErr != nil {
		return nil, titleFontErr
	}
	return opentype.NewFace(titleFontParsed, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func bodyFontFace(size float64) (font.Face, error) {
	bodyFontOnce.Do(func() {
		bodyFontParsed, bodyFontErr = opentype.Parse(embeddedBodyFont)
	})
	if bodyFontErr != nil {
		return nil, bodyFontErr
	}
	return opentype.NewFace(bodyFontParsed, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func toneColor(t Tone, m theme) color.Color {
	switch t {
	case ToneWarm:
		return m.warm
	case ToneDanger:
		return m.danger
	case ToneGood:
		return m.good
	default:
		return m.title
	}
}

func newTheme() theme {
	return theme{
		background:   hexColor("#000000"),
		cardTop:      hexColor("#000000"),
		cardBottom:   hexColor("#000000"),
		cardStroke:   rgbaColor(212, 175, 88, 96),
		topLine:      rgbaColor(241, 210, 124, 132),
		divider:      rgbaColor(216, 173, 79, 74),
		text:         hexColor("#fffaf0"),
		textSoft:     hexColor("#d8cfbb"),
		textMute:     hexColor("#b8ad95"),
		title:        hexColor("#ffffff"),
		accent:       hexColor("#f1d27c"),
		chipBg:       hexColor("#000000"),
		chipStroke:   rgbaColor(216, 173, 79, 132),
		chipText:     hexColor("#f1d27c"),
		panelTop:     hexColor("#000000"),
		panelBottom:  hexColor("#000000"),
		panelStroke:  rgbaColor(216, 173, 79, 86),
		barTrack:     rgbaColor(216, 173, 79, 38),
		good:         hexColor("#7bd8a4"),
		warm:         hexColor("#f3c36f"),
		danger:       hexColor("#ff877e"),
		actionHint:   hexColor("#c9bea9"),
		qrWrapBg:     hexColor("#020202"),
		qrWrapStroke: rgbaColor(216, 173, 79, 132),
		qrCodeBg:     hexColor("#ffffff"),
	}
}

func clampMetricPercent(percent float64) float64 {
	switch {
	case math.IsNaN(percent), math.IsInf(percent, 0), percent < 0:
		return 0
	case percent > 100:
		return 100
	default:
		return percent
	}
}

func hexColor(value string) color.Color {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return color.RGBA{0, 0, 0, 255}
	}
	var rgb [3]uint8
	for index := 0; index < 3; index++ {
		component := value[index*2 : index*2+2]
		var parsed uint64
		for _, ch := range component {
			parsed <<= 4
			switch {
			case ch >= '0' && ch <= '9':
				parsed += uint64(ch - '0')
			case ch >= 'a' && ch <= 'f':
				parsed += uint64(ch-'a') + 10
			case ch >= 'A' && ch <= 'F':
				parsed += uint64(ch-'A') + 10
			}
		}
		rgb[index] = uint8(parsed)
	}
	return color.RGBA{rgb[0], rgb[1], rgb[2], 255}
}

func rgbaColor(r, g, b, a int) color.Color {
	return color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
}
