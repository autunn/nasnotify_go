package nas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func formatUGreenSpeed(bytesPerSec float64) (string, string) {
	if bytesPerSec >= 1024*1024 {
		return fmt.Sprintf("%.1fMB/s", bytesPerSec/1024/1024), "MB/s"
	}
	return fmt.Sprintf("%.1fKB/s", bytesPerSec/1024), "KB/s"
}

func getLastUGreenTime(file string) int64 {
	content, err := os.ReadFile(file)
	if err != nil {
		return 0
	}
	var maxTime int64
	for _, line := range strings.Split(string(content), "\n") {
		parts := strings.SplitN(line, "：", 2)
		if len(parts) == 2 {
			if t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(parts[0]), time.Local); err == nil {
				if t.Unix() > maxTime {
					maxTime = t.Unix()
				}
			}
		}
	}
	return maxTime
}

func saveUGreenNotices(notices []UGreenNotice, file string) error {
	var builder strings.Builder
	for _, notice := range notices {
		t := time.Unix(notice.Time, 0).In(time.FixedZone("CST", 8*3600))
		builder.WriteString(fmt.Sprintf("%s：%s\n", t.Format("2006-01-02 15:04:05"), notice.Body))
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(builder.String()), 0600)
}

func buildUGreenPushContent(notices []UGreenNotice, typeName string) string {
	if len(notices) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(wechatCardHeader("🔂", "系统通知", typeName))
	builder.WriteString(fmt.Sprintf("事件数 %d\n", len(notices)))
	builder.WriteString(wechatSection("通知列表"))
	for i, notice := range notices {
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, trimDisplayText(notice.Body, 80)))
	}
	return strings.TrimSpace(builder.String())
}

func buildUGreenSystemStatusPushContent(info *UGreenSystemInfo, typeName string) string {
	if info == nil {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(wechatCardHeader("📳", "系统状态概览", typeName))
	builder.WriteString(wechatPercentLine("CPU", info.UsageCpu))
	if info.CpuTemp > 0 {
		builder.WriteString(fmt.Sprintf("  温度 %.1f°C", info.CpuTemp))
	}
	builder.WriteString("\n")
	if info.CpuFan > 0 {
		builder.WriteString(fmt.Sprintf("CPU风扇 %d RPM\n", info.CpuFan))
	}
	if info.DeviceFan > 0 {
		builder.WriteString(fmt.Sprintf("系统风扇 %d RPM\n", info.DeviceFan))
	}
	memUsedGB := float64(info.MemoryUsed) / 1024 / 1024 / 1024
	memTotalGB := float64(info.MemoryTotal) / 1024 / 1024 / 1024
	builder.WriteString(wechatPercentLine("内存", info.UsageMemory))
	if info.MemoryTotal > 0 {
		builder.WriteString(fmt.Sprintf("  %.1fG/%.1fG", memUsedGB, memTotalGB))
	}
	builder.WriteString("\n")
	builder.WriteString(wechatSection("网络速率"))
	builder.WriteString(fmt.Sprintf("下载  %s\n", fallbackText(info.NetworkReceive, "0 KB/s")))
	builder.WriteString(fmt.Sprintf("上传  %s\n", fallbackText(info.NetworkTransmit, "0 KB/s")))
	if len(info.Storage) > 0 {
		builder.WriteString(wechatSection("存储概览"))
		for i, item := range info.Storage {
			if i >= 3 {
				builder.WriteString(fmt.Sprintf("...另有 %d 个存储项\n", len(info.Storage)-i))
				break
			}
			total := item.Size
			used := item.Used
			usagePct := 0.0
			if total > 0 {
				usagePct = float64(used) / float64(total) * 100
			}
			name := fallbackText(item.StorageName, fallbackText(item.Name, "未命名存储"))
			builder.WriteString(fmt.Sprintf("%s\n", trimDisplayText(name, 24)))
			builder.WriteString(wechatPercentLine("已用", usagePct) + "\n")
		}
	}
	return strings.TrimSpace(builder.String())
}
