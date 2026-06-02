package nas

import (
	"fmt"
	"net"
	"strings"
	"time"

	"nasnotify-go/internal/notify"
	"nasnotify-go/internal/notifycard"
	"nasnotify-go/internal/utils"
)

func PushUGreenHealthCheck() {
	cfg := configuredUGreenDevice()
	if cfg == nil {
		notify.WechatPushCard(
			notifycard.Card{
				Title:   "绿联 NAS 巡检",
				Device:  "未配置",
				Summary: "后台还没有配置本机绿联 NAS 管理账号，无法执行巡检。",
				Badges:  []string{"配置缺失", "需要处理"},
				Metrics: []notifycard.Metric{
					{Label: "配置状态", Value: "未完成", Hint: "请在基础设置里填写 NAS 地址、账号和密码", Tone: notifycard.ToneDanger},
				},
			},
			"绿联 NAS 巡检失败：后台还没有配置本机绿联 NAS 管理账号。",
		)
		return
	}

	ip, port := utils.SplitIpPort(cfg.IpPort, 9999)
	deviceLabel := ugreenDeviceLabel(*cfg)
	tcpOK, tcpHint := checkTCPReachable(ip, port)

	loginOK := false
	loginHint := "未尝试登录"
	var info *UGreenSystemInfo
	if tcpOK {
		authInfo, err := ensureAuthWithError(cfg.Username, cfg.Password, ip, port, cfg.UseSSL)
		if err != nil || authInfo == nil {
			loginHint = formatUGreenAuthError(err)
		} else {
			loginOK = true
			loginHint = "管理接口登录正常"
			if fetched, fetchErr := fetchUGreenSystemInfo(authInfo, ip, port, cfg.UseSSL); fetchErr == nil {
				info = fetched
			} else {
				loginHint = "登录正常，状态接口读取失败：" + fetchErr.Error()
			}
		}
	}

	wolReady := strings.TrimSpace(cfg.MacAddress) != ""
	card := notifycard.Card{
		Title:   "绿联 NAS 巡检",
		Device:  deviceLabel,
		Summary: healthSummary(tcpOK, loginOK, wolReady),
		Badges:  healthBadges(tcpOK, loginOK, wolReady),
		Metrics: []notifycard.Metric{
			{Label: "接口连通", Value: statusText(tcpOK), Hint: fmt.Sprintf("%s:%d · %s", ip, port, tcpHint), Tone: healthBoolTone(tcpOK)},
			{Label: "登录状态", Value: statusText(loginOK), Hint: loginHint, Tone: healthBoolTone(loginOK)},
			{Label: "远程唤醒", Value: wolStatusText(wolReady), Hint: wolHint(wolReady), Tone: wolTone(wolReady)},
		},
		Sections: []notifycard.Section{
			{
				Title: "建议动作",
				Lines: healthAdvice(tcpOK, loginOK, wolReady),
			},
		},
	}

	if info != nil {
		card.Metrics = append(card.Metrics,
			notifycard.Metric{
				Label: "CPU",
				Value: fmt.Sprintf("%.0f%%", clampPercent(info.UsageCpu)),
				Hint:  temperatureHint(info.CpuTemp),
				Tone:  usageTone(info.UsageCpu),
				Chart: &notifycard.MetricChart{Percent: clampPercent(info.UsageCpu)},
			},
			notifycard.Metric{
				Label: "内存",
				Value: fmt.Sprintf("%.0f%%", clampPercent(info.UsageMemory)),
				Hint:  memoryHint(info),
				Tone:  usageTone(info.UsageMemory),
				Chart: &notifycard.MetricChart{Percent: clampPercent(info.UsageMemory)},
			},
		)
	}

	notify.WechatPushCard(card, healthFallbackText(deviceLabel, ip, port, tcpOK, loginOK, wolReady, tcpHint, loginHint))
}

func checkTCPReachable(ip string, port int) (bool, string) {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", address, 2500*time.Millisecond)
	if err != nil {
		return false, err.Error()
	}
	_ = conn.Close()
	return true, "TCP 连接正常"
}

func healthSummary(tcpOK, loginOK, wolReady bool) string {
	if tcpOK && loginOK && wolReady {
		return "核心链路正常，管理接口可登录，远程唤醒配置已就绪。"
	}
	if tcpOK && loginOK {
		return "管理接口可正常访问，远程唤醒尚未配置 MAC 地址。"
	}
	if tcpOK {
		return "NAS 地址可达，但管理账号登录失败或状态接口不可用。"
	}
	return "当前无法连接 NAS 管理端口，请优先检查地址、端口和网络。"
}

func healthBadges(tcpOK, loginOK, wolReady bool) []string {
	return []string{
		"接口" + statusText(tcpOK),
		"登录" + statusText(loginOK),
		"WOL" + wolStatusText(wolReady),
	}
}

func healthAdvice(tcpOK, loginOK, wolReady bool) []string {
	lines := make([]string, 0, 4)
	if !tcpOK {
		lines = append(lines, "确认 NAS 地址 / IP 和端口是否正确，Docker 或 macOS 版本通常需要填写真实内网 IP。")
	}
	if tcpOK && !loginOK {
		lines = append(lines, "确认本机 NAS 管理账号和密码是否正确，必要时在后台重新保存密码。")
	}
	if !wolReady {
		lines = append(lines, "如需使用“唤醒”指令，请在基础设置里填写 NAS MAC 地址。")
	}
	if len(lines) == 0 {
		lines = append(lines, "配置状态良好，可继续使用状态、存储、Docker、进程、备份、电源和 UPS 指令。")
	}
	return lines
}

func healthFallbackText(deviceLabel, ip string, port int, tcpOK, loginOK, wolReady bool, tcpHint, loginHint string) string {
	return fmt.Sprintf(
		"绿联 NAS 巡检\n\n设备：%s\n地址：%s:%d\n接口：%s（%s）\n登录：%s（%s）\n远程唤醒：%s",
		deviceLabel,
		ip,
		port,
		statusText(tcpOK),
		tcpHint,
		statusText(loginOK),
		loginHint,
		wolStatusText(wolReady),
	)
}

func statusText(ok bool) string {
	if ok {
		return "正常"
	}
	return "异常"
}

func wolStatusText(ok bool) string {
	if ok {
		return "已配置"
	}
	return "未配置"
}

func wolHint(ok bool) string {
	if ok {
		return "可使用 唤醒 / wol 指令"
	}
	return "填写 MAC 地址后启用"
}

func healthBoolTone(ok bool) notifycard.Tone {
	if ok {
		return notifycard.ToneGood
	}
	return notifycard.ToneDanger
}

func wolTone(ok bool) notifycard.Tone {
	if ok {
		return notifycard.ToneGood
	}
	return notifycard.ToneWarm
}

func usageTone(percent float64) notifycard.Tone {
	switch {
	case percent >= 85:
		return notifycard.ToneDanger
	case percent >= 70:
		return notifycard.ToneWarm
	default:
		return notifycard.ToneGood
	}
}

func temperatureHint(temp float64) string {
	if temp <= 0 {
		return "温度未返回"
	}
	return fmt.Sprintf("温度 %.0f°C", temp)
}

func memoryHint(info *UGreenSystemInfo) string {
	if info == nil || info.MemoryTotal <= 0 {
		return "容量未返回"
	}
	return fmt.Sprintf("%s / %s", formatBytesHuman(info.MemoryUsed), formatBytesHuman(info.MemoryTotal))
}
