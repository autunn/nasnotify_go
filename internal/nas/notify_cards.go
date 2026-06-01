package nas

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"nasnotify-go/internal/notifycard"
)

type ugreenStorageCardVolume struct {
	Label      string
	PoolName   string
	FileSystem string
	Total      int64
	Used       int64
}

type ugreenUPSCardState struct {
	Connected      bool
	ConnectionType string
	Supplier       string
	ProductModel   string
	StatusText     string
	BatteryText    string
	BatteryPercent float64
	HasBatteryPct  bool
	EnduranceText  string
	InputStatus    string
	ProtectPolicy  string
	DelayText      string
}

type ugreenDockerCardState struct {
	RunningCount int
	TotalCount   int
	ImageCount   int
	CPUUsed      float64
	RunningNames []string
}

type ugreenProcessCardItem struct {
	Name   string
	CPU    float64
	Memory float64
}

type ugreenBackupCardItem struct {
	Name     string
	Status   string
	LastSync string
	Tone     notifycard.Tone
}

type ugreenPowerCardState struct {
	PowerBoot      bool
	WakeOn         bool
	HardDriveSleep bool
	HardDriveAfter string
}

func buildUGreenSystemStatusCard(info *UGreenSystemInfo, typeName string) notifycard.Card {
	device := fallbackText(typeName, "绿联 NAS")
	if info == nil {
		return notifycard.Card{
			Title:   "系统状态概览",
			Device:  device,
			Summary: "当前未获取到可展示的系统状态数据。",
		}
	}

	if name := strings.TrimSpace(info.System.DevName); name != "" {
		device = fallbackText(typeName, name)
	}

	summary := joinCardSummary(
		fmt.Sprintf("CPU %s", formatPercentMetric(info.UsageCpu)),
		fmt.Sprintf("温度 %s", formatTempMetric(info.CpuTemp)),
		fmt.Sprintf("内存 %s", formatPercentMetric(info.UsageMemory)),
		fmt.Sprintf("下载 %s", fallbackText(info.NetworkReceive, "0 KB/s")),
		fmt.Sprintf("上传 %s", fallbackText(info.NetworkTransmit, "0 KB/s")),
	)

	card := notifycard.Card{
		Title:   "系统状态概览",
		Device:  device,
		Summary: summary,
		Badges: uniqueCardBadges(
			strings.TrimSpace(info.System.SystemVersion),
			metricBadge(len(info.System.NetworkInfo), "网口"),
			metricBadge(len(info.Storage), "存储项"),
		),
		Metrics: []notifycard.Metric{
			{
				Label: "CPU 占用",
				Value: formatPercentMetric(info.UsageCpu),
				Hint:  formatFanHint("CPU 风扇", info.CpuFan),
				Tone:  toneFromPercent(info.UsageCpu),
				Chart: metricChart(info.UsageCpu),
			},
			{
				Label: "CPU 温度",
				Value: formatTempMetric(info.CpuTemp),
				Hint:  "处理器实时温度",
				Tone:  temperatureTone(info.CpuTemp),
				Chart: temperatureMetricChart(info.CpuTemp),
			},
			{
				Label: "内存占用",
				Value: formatPercentMetric(info.UsageMemory),
				Hint:  formatMemoryHint(info.MemoryUsed, info.MemoryTotal),
				Tone:  toneFromPercent(info.UsageMemory),
				Chart: metricChart(info.UsageMemory),
			},
			{
				Label: "下载速率",
				Value: fallbackText(info.NetworkReceive, "0 KB/s"),
				Hint:  "当前网络接收",
				Tone:  notifycard.ToneDefault,
			},
			{
				Label: "上传速率",
				Value: fallbackText(info.NetworkTransmit, "0 KB/s"),
				Hint:  "当前网络发送",
				Tone:  notifycard.ToneDefault,
			},
		},
	}

	infoLines := []string{}
	if version := strings.TrimSpace(info.System.SystemVersion); version != "" {
		infoLines = append(infoLines, "系统版本："+version)
	}
	if bootTime := formatUGreenBootTime(info.System); bootTime != "" {
		infoLines = append(infoLines, "最近启动："+bootTime)
	}
	if msg := strings.TrimSpace(info.System.Message); msg != "" {
		infoLines = append(infoLines, "系统消息："+msg)
	}
	if info.DeviceFan > 0 {
		infoLines = append(infoLines, fmt.Sprintf("系统风扇：%d RPM", info.DeviceFan))
	}
	if info.CpuTemp > 0 {
		infoLines = append(infoLines, fmt.Sprintf("CPU 温度：%s", formatTempMetric(info.CpuTemp)))
	}
	if len(infoLines) > 0 {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "设备信息",
			Lines: infoLines,
		})
	}

	networkLines := make([]string, 0, len(info.System.NetworkInfo))
	for _, item := range info.System.NetworkInfo {
		label := fallbackText(item.Label, "网络接口")
		switch {
		case strings.TrimSpace(item.IPv4) != "" && strings.TrimSpace(item.IPv6) != "":
			networkLines = append(networkLines, fmt.Sprintf("%s：%s / %s", label, strings.TrimSpace(item.IPv4), strings.TrimSpace(item.IPv6)))
		case strings.TrimSpace(item.IPv4) != "":
			networkLines = append(networkLines, fmt.Sprintf("%s：%s", label, strings.TrimSpace(item.IPv4)))
		case strings.TrimSpace(item.IPv6) != "":
			networkLines = append(networkLines, fmt.Sprintf("%s：%s", label, strings.TrimSpace(item.IPv6)))
		}
	}
	if lines, hidden := limitCardLines(networkLines, 4); len(lines) > 0 {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "网络接口",
			Lines: lines,
		})
		if hidden > 0 {
			card.Footer = appendCardFooter(card.Footer, fmt.Sprintf("另有 %d 个网口未展开。", hidden))
		}
	}

	storageLines := make([]string, 0, len(info.Storage))
	for _, item := range info.Storage {
		total := item.Size
		used := item.Used
		usagePct := 0.0
		if total > 0 {
			usagePct = float64(used) / float64(total) * 100
		}
		name := fallbackText(item.StorageName, fallbackText(item.Name, "未命名存储"))
		line := fmt.Sprintf("%s · %s / %s · %s", name, formatBytesHuman(used), formatBytesHuman(total), formatPercentMetric(usagePct))
		storageLines = append(storageLines, line)
	}
	if lines, hidden := limitCardLines(storageLines, 4); len(lines) > 0 {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "存储概览",
			Lines: lines,
		})
		if hidden > 0 {
			card.Footer = appendCardFooter(card.Footer, fmt.Sprintf("另有 %d 个存储项未展开。", hidden))
		}
	}

	return card
}

func buildUGreenNoticeCard(notices []UGreenNotice, typeName string) notifycard.Card {
	device := fallbackText(typeName, "绿联 NAS")
	alertCount := 0
	lines := make([]string, 0, len(notices))
	for _, notice := range notices {
		body := trimDisplayText(strings.TrimSpace(notice.Body), 80)
		if body == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  %s", formatUGreenTimestamp(notice.Time), body))
		if noticeNeedsAttention(notice.Body) {
			alertCount++
		}
	}

	displayLines, hidden := limitCardLines(lines, 6)
	card := notifycard.Card{
		Title:   "系统通知",
		Device:  device,
		Summary: joinCardSummary(fmt.Sprintf("最近共收到 %d 条系统通知", len(lines)), noticeSummary(alertCount)),
		Badges: uniqueCardBadges(
			metricBadge(len(lines), "条通知"),
			attentionBadge(alertCount),
		),
		Metrics: []notifycard.Metric{
			{
				Label: "通知数量",
				Value: fmt.Sprintf("%d 条", len(lines)),
				Hint:  "最近一次巡检结果",
				Tone:  notifycard.ToneGood,
			},
			{
				Label: "关注事项",
				Value: fmt.Sprintf("%d 条", alertCount),
				Hint:  "按异常/失败等关键词识别",
				Tone:  attentionTone(alertCount),
				Chart: metricChart(percentOfCount(alertCount, maxInt(len(lines), 1))),
			},
			{
				Label: "最新时间",
				Value: latestNoticeTime(notices),
				Hint:  "最近一条通知时间",
				Tone:  notifycard.ToneDefault,
			},
			{
				Label: "展示条数",
				Value: fmt.Sprintf("%d 条", len(displayLines)),
				Hint:  "图片卡片内实际展开数量",
				Tone:  notifycard.ToneDefault,
			},
		},
	}
	if len(displayLines) > 0 {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "通知列表",
			Lines: displayLines,
		})
	}
	if hidden > 0 {
		card.Footer = appendCardFooter(card.Footer, fmt.Sprintf("另有 %d 条通知未展开。", hidden))
	}
	return card
}

func buildUGreenStorageCard(volumes []ugreenStorageCardVolume, typeName string) notifycard.Card {
	device := fallbackText(typeName, "绿联 NAS")
	totalUsed := int64(0)
	totalSize := int64(0)
	lines := make([]string, 0, len(volumes))
	for _, volume := range volumes {
		totalUsed += volume.Used
		totalSize += volume.Total
		usagePct := 0.0
		if volume.Total > 0 {
			usagePct = float64(volume.Used) / float64(volume.Total) * 100
		}
		name := fallbackText(volume.Label, "未命名存储卷")
		meta := []string{}
		if pool := strings.TrimSpace(volume.PoolName); pool != "" {
			meta = append(meta, pool)
		}
		if fs := strings.TrimSpace(volume.FileSystem); fs != "" {
			meta = append(meta, fs)
		}
		suffix := ""
		if len(meta) > 0 {
			suffix = " · " + strings.Join(meta, " / ")
		}
		lines = append(lines, fmt.Sprintf("%s%s · %s / %s · %s", name, suffix, formatBytesHuman(volume.Used), formatBytesHuman(volume.Total), formatPercentMetric(usagePct)))
	}

	totalPct := 0.0
	if totalSize > 0 {
		totalPct = float64(totalUsed) / float64(totalSize) * 100
	}
	displayLines, hidden := limitCardLines(lines, 6)
	card := notifycard.Card{
		Title:   "存储卷状态",
		Device:  device,
		Summary: joinCardSummary(fmt.Sprintf("已识别 %d 个存储卷", len(volumes)), fmt.Sprintf("总使用率 %s", formatPercentMetric(totalPct))),
		Badges:  uniqueCardBadges(metricBadge(len(volumes), "个卷")),
		Metrics: []notifycard.Metric{
			{
				Label: "存储卷",
				Value: fmt.Sprintf("%d 个", len(volumes)),
				Hint:  "当前识别到的卷数量",
				Tone:  notifycard.ToneGood,
			},
			{
				Label: "总使用率",
				Value: formatPercentMetric(totalPct),
				Hint:  fmt.Sprintf("%s / %s", formatBytesHuman(totalUsed), formatBytesHuman(totalSize)),
				Tone:  toneFromPercent(totalPct),
				Chart: metricChart(totalPct),
			},
			{
				Label: "已用空间",
				Value: formatBytesHuman(totalUsed),
				Hint:  "所有卷已用容量合计",
				Tone:  notifycard.ToneDefault,
			},
			{
				Label: "总容量",
				Value: formatBytesHuman(totalSize),
				Hint:  "所有卷总容量合计",
				Tone:  notifycard.ToneDefault,
			},
		},
	}
	if len(displayLines) > 0 {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "卷详情",
			Lines: displayLines,
		})
	}
	if hidden > 0 {
		card.Footer = appendCardFooter(card.Footer, fmt.Sprintf("另有 %d 个存储卷未展开。", hidden))
	}
	return card
}

func buildUGreenUPSCard(state ugreenUPSCardState, typeName string) notifycard.Card {
	device := fallbackText(typeName, "绿联 NAS")
	summary := "当前未检测到已连接的 UPS 设备。"
	if state.Connected {
		summary = joinCardSummary(
			fmt.Sprintf("%s %s 已连接", fallbackText(state.Supplier, "UPS"), strings.TrimSpace(state.ProductModel)),
			fallbackText(state.InputStatus, "状态已同步"),
		)
	}

	card := notifycard.Card{
		Title:   "UPS 供电状态",
		Device:  device,
		Summary: summary,
		Badges: uniqueCardBadges(
			strings.TrimSpace(state.ConnectionType),
			connectionBadge(state.Connected),
		),
		Metrics: []notifycard.Metric{
			{
				Label: "连接状态",
				Value: connectionStatusText(state.Connected),
				Hint:  joinHintIfAny(strings.TrimSpace(state.Supplier), strings.TrimSpace(state.ProductModel)),
				Tone:  connectionStatusTone(state.Connected),
				Chart: metricChart(boolPercent(state.Connected)),
			},
			{
				Label: "电池电量",
				Value: fallbackText(state.BatteryText, "未知"),
				Hint:  "当前电池电量",
				Tone:  batteryTone(state),
				Chart: batteryMetricChart(state),
			},
			{
				Label: "续航预估",
				Value: fallbackText(state.EnduranceText, "未知"),
				Hint:  fallbackText(state.InputStatus, "供电状态未知"),
				Tone:  notifycard.ToneDefault,
			},
			{
				Label: "保护策略",
				Value: fallbackText(state.ProtectPolicy, "未配置"),
				Hint:  fallbackText(state.DelayText, "未设置延时"),
				Tone:  notifycard.ToneDefault,
			},
		},
	}

	deviceLines := []string{}
	if text := strings.TrimSpace(state.Supplier); text != "" || strings.TrimSpace(state.ProductModel) != "" {
		deviceLines = append(deviceLines, fmt.Sprintf("设备：%s %s", strings.TrimSpace(state.Supplier), strings.TrimSpace(state.ProductModel)))
	}
	if text := strings.TrimSpace(state.ConnectionType); text != "" {
		deviceLines = append(deviceLines, "连接方式："+text)
	}
	if text := strings.TrimSpace(state.StatusText); text != "" {
		deviceLines = append(deviceLines, "UPS 状态："+text)
	}
	if text := strings.TrimSpace(state.InputStatus); text != "" {
		deviceLines = append(deviceLines, "供电状态："+text)
	}
	if len(deviceLines) > 0 {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "设备信息",
			Lines: deviceLines,
		})
	}

	policyLines := []string{}
	if text := strings.TrimSpace(state.ProtectPolicy); text != "" {
		policyLines = append(policyLines, "保护策略："+text)
	}
	if text := strings.TrimSpace(state.DelayText); text != "" {
		policyLines = append(policyLines, "延时设置："+text)
	}
	if len(policyLines) > 0 {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "保护设置",
			Lines: policyLines,
		})
	}
	return card
}

func buildUGreenDockerCard(state ugreenDockerCardState, typeName string) notifycard.Card {
	device := fallbackText(typeName, "绿联 NAS")
	displayLines, hidden := limitCardLines(state.RunningNames, 8)
	card := notifycard.Card{
		Title:   "Docker 运行概览",
		Device:  device,
		Summary: joinCardSummary(fmt.Sprintf("%d / %d 个容器运行中", state.RunningCount, state.TotalCount), fmt.Sprintf("镜像 %d 个", state.ImageCount)),
		Badges:  uniqueCardBadges(metricBadge(state.RunningCount, "个运行中容器")),
		Metrics: []notifycard.Metric{
			{
				Label: "运行容器",
				Value: fmt.Sprintf("%d / %d", state.RunningCount, state.TotalCount),
				Hint:  "运行数 / 总数",
				Tone:  countTone(state.RunningCount),
				Chart: metricChart(percentOfCount(state.RunningCount, maxInt(state.TotalCount, 1))),
			},
			{
				Label: "CPU 负载",
				Value: formatPercentMetric(state.CPUUsed),
				Hint:  "当前 Docker CPU 占用",
				Tone:  toneFromPercent(state.CPUUsed),
				Chart: metricChart(state.CPUUsed),
			},
			{
				Label: "镜像数量",
				Value: fmt.Sprintf("%d 个", state.ImageCount),
				Hint:  "本机已拉取镜像",
				Tone:  notifycard.ToneDefault,
			},
			{
				Label: "展示列表",
				Value: fmt.Sprintf("%d 个", len(displayLines)),
				Hint:  "本卡片已展开数量",
				Tone:  notifycard.ToneDefault,
			},
		},
	}
	if len(displayLines) > 0 {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "运行中容器",
			Lines: displayLines,
		})
	} else {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "运行中容器",
			Lines: []string{"当前没有运行中的容器。"},
		})
	}
	if hidden > 0 {
		card.Footer = appendCardFooter(card.Footer, fmt.Sprintf("另有 %d 个运行中容器未展开。", hidden))
	}
	return card
}

func buildUGreenProcessCard(items []ugreenProcessCardItem, typeName string) notifycard.Card {
	device := fallbackText(typeName, "绿联 NAS")
	topCPU := ugreenProcessCardItem{}
	topMemory := ugreenProcessCardItem{}
	if len(items) > 0 {
		topCPU = items[0]
		topMemory = items[0]
		for _, item := range items {
			if item.Memory > topMemory.Memory {
				topMemory = item
			}
		}
	}

	lines := make([]string, 0, len(items))
	for index, item := range items {
		if index >= 5 {
			break
		}
		lines = append(lines, fmt.Sprintf("%s · CPU %s · 内存 %s", trimDisplayText(item.Name, 28), formatPercentMetric(item.CPU), formatPercentMetric(item.Memory)))
	}

	card := notifycard.Card{
		Title:   "进程占用 TOP 5",
		Device:  device,
		Summary: joinCardSummary(fmt.Sprintf("共抓取到 %d 个服务 / 进程样本", len(items)), "已按 CPU 占用从高到低排序"),
		Metrics: []notifycard.Metric{
			{
				Label: "进程总数",
				Value: fmt.Sprintf("%d 个", len(items)),
				Hint:  "服务列表与进程列表合并后统计",
				Tone:  notifycard.ToneDefault,
			},
			{
				Label: "最高 CPU",
				Value: formatPercentMetric(topCPU.CPU),
				Hint:  trimDisplayText(topCPU.Name, 24),
				Tone:  toneFromPercent(topCPU.CPU),
				Chart: metricChart(topCPU.CPU),
			},
			{
				Label: "最高内存",
				Value: formatPercentMetric(topMemory.Memory),
				Hint:  trimDisplayText(topMemory.Name, 24),
				Tone:  toneFromPercent(topMemory.Memory),
				Chart: metricChart(topMemory.Memory),
			},
			{
				Label: "展示数量",
				Value: fmt.Sprintf("%d 条", len(lines)),
				Hint:  "默认只展开前 5 名",
				Tone:  notifycard.ToneGood,
			},
		},
	}
	if len(lines) > 0 {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "进程列表",
			Lines: lines,
		})
	} else {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "进程列表",
			Lines: []string{"当前未获取到进程数据。"},
		})
	}
	if len(items) > len(lines) {
		card.Footer = appendCardFooter(card.Footer, fmt.Sprintf("其余 %d 个进程未展示。", len(items)-len(lines)))
	}
	return card
}

func buildUGreenBackupCard(items []ugreenBackupCardItem, typeName string) notifycard.Card {
	device := fallbackText(typeName, "绿联 NAS")
	running := 0
	paused := 0
	abnormal := 0
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("%s · %s · %s", trimDisplayText(item.Name, 28), fallbackText(item.Status, "未知"), fallbackText(item.LastSync, "从未运行")))
		switch {
		case strings.Contains(item.Status, "运行"):
			running++
		case strings.Contains(item.Status, "暂停"):
			paused++
		case strings.Contains(item.Status, "异常"):
			abnormal++
		}
	}
	displayLines, hidden := limitCardLines(lines, 6)
	card := notifycard.Card{
		Title:   "备份任务状态",
		Device:  device,
		Summary: joinCardSummary(fmt.Sprintf("当前共配置 %d 个备份任务", len(items)), fmt.Sprintf("运行中 %d 个，异常 %d 个", running, abnormal)),
		Metrics: []notifycard.Metric{
			{
				Label: "任务总数",
				Value: fmt.Sprintf("%d 个", len(items)),
				Hint:  "已配置的备份任务",
				Tone:  notifycard.ToneDefault,
			},
			{
				Label: "运行中",
				Value: fmt.Sprintf("%d 个", running),
				Hint:  "当前正在执行或同步中",
				Tone:  countTone(running),
				Chart: metricChart(percentOfCount(running, maxInt(len(items), 1))),
			},
			{
				Label: "异常任务",
				Value: fmt.Sprintf("%d 个", abnormal),
				Hint:  "需要优先关注",
				Tone:  attentionTone(abnormal),
				Chart: metricChart(percentOfCount(abnormal, maxInt(len(items), 1))),
			},
			{
				Label: "已暂停",
				Value: fmt.Sprintf("%d 个", paused),
				Hint:  "暂停不会自动继续执行",
				Tone:  countTone(paused),
				Chart: metricChart(percentOfCount(paused, maxInt(len(items), 1))),
			},
		},
	}
	if len(displayLines) > 0 {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "任务列表",
			Lines: displayLines,
		})
	} else {
		card.Sections = append(card.Sections, notifycard.Section{
			Title: "任务列表",
			Lines: []string{"当前没有配置备份任务。"},
		})
	}
	if hidden > 0 {
		card.Footer = appendCardFooter(card.Footer, fmt.Sprintf("另有 %d 个备份任务未展开。", hidden))
	}
	return card
}

func buildUGreenPowerCard(state ugreenPowerCardState, typeName string) notifycard.Card {
	device := fallbackText(typeName, "绿联 NAS")
	card := notifycard.Card{
		Title:   "电源与休眠配置",
		Device:  device,
		Summary: joinCardSummary("当前展示的是 NAS 电源恢复、网络唤醒与磁盘休眠配置", "适合做远程值守前的快速检查"),
		Metrics: []notifycard.Metric{
			boolMetric("来电开机", state.PowerBoot, "断电恢复后是否自动启动"),
			boolMetric("网络唤醒", state.WakeOn, "WOL 远程唤醒开关"),
			boolMetric("磁盘休眠", state.HardDriveSleep, "磁盘空闲后是否进入休眠"),
			{
				Label: "休眠时间",
				Value: fallbackText(state.HardDriveAfter, "未启用"),
				Hint:  "磁盘空闲后进入休眠的时间",
				Tone:  boolTone(state.HardDriveSleep),
			},
		},
		Sections: []notifycard.Section{
			{
				Title: "配置明细",
				Lines: []string{
					"来电开机：" + boolDisplay(state.PowerBoot),
					"网络唤醒：" + boolDisplay(state.WakeOn),
					"磁盘休眠：" + boolDisplay(state.HardDriveSleep),
					"休眠时间：" + fallbackText(state.HardDriveAfter, "未启用"),
				},
			},
		},
	}
	return card
}

func joinCardSummary(parts ...string) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, "，") + "。"
}

func uniqueCardBadges(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func appendCardFooter(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	switch {
	case current == "":
		return next
	case next == "":
		return current
	default:
		return current + " " + next
	}
}

func limitCardLines(lines []string, limit int) ([]string, int) {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	if limit <= 0 || len(filtered) <= limit {
		return filtered, 0
	}
	return filtered[:limit], len(filtered) - limit
}

func formatPercentMetric(percent float64) string {
	return fmt.Sprintf("%.0f%%", clampPercent(percent))
}

func toneFromPercent(percent float64) notifycard.Tone {
	percent = clampPercent(percent)
	switch {
	case percent >= 90:
		return notifycard.ToneDanger
	case percent >= 70:
		return notifycard.ToneWarm
	default:
		return notifycard.ToneGood
	}
}

func boolMetric(label string, enabled bool, hint string) notifycard.Metric {
	return notifycard.Metric{
		Label: label,
		Value: boolDisplay(enabled),
		Hint:  hint,
		Tone:  boolTone(enabled),
		Chart: metricChart(boolPercent(enabled)),
	}
}

func boolDisplay(enabled bool) string {
	if enabled {
		return "开启"
	}
	return "关闭"
}

func boolTone(enabled bool) notifycard.Tone {
	if enabled {
		return notifycard.ToneGood
	}
	return notifycard.ToneDefault
}

func boolPercent(enabled bool) float64 {
	if enabled {
		return 100
	}
	return 0
}

func countTone(count int) notifycard.Tone {
	if count > 0 {
		return notifycard.ToneGood
	}
	return notifycard.ToneDefault
}

func attentionTone(count int) notifycard.Tone {
	switch {
	case count >= 3:
		return notifycard.ToneDanger
	case count > 0:
		return notifycard.ToneWarm
	default:
		return notifycard.ToneGood
	}
}

func noticeNeedsAttention(body string) bool {
	body = strings.ToLower(strings.TrimSpace(body))
	for _, keyword := range []string{"失败", "异常", "错误", "告警", "警报", "离线", "损坏", "告急"} {
		if strings.Contains(body, keyword) {
			return true
		}
	}
	return false
}

func noticeSummary(alertCount int) string {
	switch {
	case alertCount <= 0:
		return "当前没有识别到明显的异常关键词"
	case alertCount == 1:
		return "其中 1 条需要关注"
	default:
		return fmt.Sprintf("其中 %d 条需要关注", alertCount)
	}
}

func latestNoticeTime(notices []UGreenNotice) string {
	latest := int64(0)
	for _, notice := range notices {
		if notice.Time > latest {
			latest = notice.Time
		}
	}
	if latest == 0 {
		return "暂无"
	}
	return formatUGreenTimestamp(latest)
}

func metricBadge(count int, suffix string) string {
	if count <= 0 {
		return ""
	}
	return fmt.Sprintf("%d %s", count, suffix)
}

func attentionBadge(alertCount int) string {
	if alertCount <= 0 {
		return "无异常关键词"
	}
	return fmt.Sprintf("%d 条需关注", alertCount)
}

func connectionBadge(connected bool) string {
	if connected {
		return "已连接"
	}
	return "未连接"
}

func connectionStatusText(connected bool) string {
	if connected {
		return "已连接"
	}
	return "未连接"
}

func connectionStatusTone(connected bool) notifycard.Tone {
	if connected {
		return notifycard.ToneGood
	}
	return notifycard.ToneWarm
}

func batteryTone(state ugreenUPSCardState) notifycard.Tone {
	if !state.HasBatteryPct {
		if state.Connected {
			return notifycard.ToneDefault
		}
		return notifycard.ToneWarm
	}
	switch {
	case state.BatteryPercent <= 20:
		return notifycard.ToneDanger
	case state.BatteryPercent <= 50:
		return notifycard.ToneWarm
	default:
		return notifycard.ToneGood
	}
}

func formatMemoryHint(used, total int64) string {
	if used <= 0 || total <= 0 {
		return ""
	}
	return fmt.Sprintf("%s / %s", formatBytesHuman(used), formatBytesHuman(total))
}

func formatTempHint(temp float64) string {
	if temp <= 0 {
		return ""
	}
	return fmt.Sprintf("温度 %.1f°C", temp)
}

func formatTempMetric(temp float64) string {
	if temp <= 0 {
		return "未知"
	}
	return fmt.Sprintf("%.1f°C", temp)
}

func temperatureTone(temp float64) notifycard.Tone {
	switch {
	case temp <= 0:
		return notifycard.ToneDefault
	case temp >= 80:
		return notifycard.ToneDanger
	case temp >= 65:
		return notifycard.ToneWarm
	default:
		return notifycard.ToneGood
	}
}

func temperatureMetricChart(temp float64) *notifycard.MetricChart {
	if temp <= 0 {
		return nil
	}
	return metricChart(temp)
}

func formatFanHint(label string, speed int) string {
	if speed <= 0 {
		return ""
	}
	return fmt.Sprintf("%s %d RPM", label, speed)
}

func joinHintIfAny(parts ...string) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}
	return strings.Join(values, " / ")
}

func formatUGreenBootTime(system UGreenSystemStatus) string {
	if text := strings.TrimSpace(system.LastBootDate); text != "" {
		return text
	}
	if system.LastBootTime <= 0 {
		return ""
	}
	return formatUGreenTimestamp(system.LastBootTime)
}

func formatUGreenTimestamp(ts int64) string {
	if ts <= 0 {
		return "未知"
	}
	if ts > 1_000_000_000_000 {
		ts /= 1000
	}
	return time.Unix(ts, 0).Format("01-02 15:04")
}

func normalizeBatteryPercent(raw string) (display string, pct float64, ok bool) {
	cleaned := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), "%"))
	if cleaned == "" {
		return "", 0, false
	}

	parsed, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return "", 0, false
	}

	switch {
	case parsed > 100 && parsed <= 1000:
		parsed = parsed / 10
	case parsed > 1000:
		parsed = 100
	}

	pct = clampPercent(parsed)
	if math.Abs(pct-math.Round(pct)) < 0.05 {
		display = fmt.Sprintf("%.0f%%", pct)
	} else {
		display = fmt.Sprintf("%.1f%%", pct)
	}
	return display, pct, true
}

func metricChart(percent float64) *notifycard.MetricChart {
	return &notifycard.MetricChart{Percent: clampPercent(percent)}
}

func batteryMetricChart(state ugreenUPSCardState) *notifycard.MetricChart {
	if !state.HasBatteryPct {
		return nil
	}
	return metricChart(state.BatteryPercent)
}

func percentOfCount(current, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(current) / float64(total) * 100
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
