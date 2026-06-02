package nas

import (
	"fmt"
	"strings"

	"nasnotify-go/internal/config"
	"nasnotify-go/internal/notify"
	"nasnotify-go/internal/utils"
)

type wakeTarget struct {
	Name string
	Mac  string
	Ip   string
}

func HandleWakeMenuCommand() {
	target, ok := localWakeTarget("")
	if !ok {
		notify.WechatPush("唤醒失败：后台没有配置本机 NAS 的 MAC 地址。\n\n请先在基础设置里填写 NAS MAC 地址。")
		return
	}
	wakeTargetDevice(target)
}

func HandleWakeCommand(targetName string) {
	targetName = strings.TrimSpace(targetName)
	target, ok := localWakeTarget(targetName)
	if !ok {
		if targetName == "" {
			notify.WechatPush("唤醒失败：后台没有配置本机 NAS 的 MAC 地址。\n\n请先在基础设置里填写 NAS MAC 地址。")
			return
		}
		notify.WechatPush(fmt.Sprintf("唤醒失败：当前只支持唤醒本机绿联 NAS，未匹配到「%s」，或后台未配置 MAC 地址。", targetName))
		return
	}
	wakeTargetDevice(target)
}

func localWakeTarget(targetName string) (wakeTarget, bool) {
	cfg := config.GetConfigSnapshot()
	name := strings.TrimSpace(cfg.LocalNasName)
	if name == "" {
		name = "本机绿联 NAS"
	}
	mac := strings.TrimSpace(cfg.LocalNasMac)
	host := strings.TrimSpace(cfg.LocalNasHost)
	if host == "" {
		host = config.DefaultLocalNasHost
	}
	if mac == "" {
		return wakeTarget{}, false
	}

	targetName = strings.TrimSpace(targetName)
	if targetName != "" {
		targetNameLower := strings.ToLower(targetName)
		nameLower := strings.ToLower(name)
		if !strings.Contains(nameLower, targetNameLower) &&
			!strings.Contains("本机绿联 nas", targetNameLower) &&
			!strings.Contains("绿联 nas", targetNameLower) {
			return wakeTarget{}, false
		}
	}

	return wakeTarget{Name: name, Mac: mac, Ip: host}, true
}

func wakeTargetDevice(t wakeTarget) {
	err := utils.WakeOnLAN(t.Mac, t.Ip)
	if err != nil {
		notify.WechatPush(fmt.Sprintf("唤醒「%s」失败：%v\nMAC: %s", t.Name, err, t.Mac))
		return
	}

	notify.WechatPush(fmt.Sprintf("唤醒指令已发出：%s\nMAC: %s", t.Name, t.Mac))
}
