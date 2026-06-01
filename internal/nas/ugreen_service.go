package nas

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"nasnotify-go/internal/config"
	"nasnotify-go/internal/notify"
	"nasnotify-go/internal/utils"
)

func configuredUGreenDevice() *config.UGreenConfig {
	cfg := config.GetConfigSnapshot()
	if local := buildLocalUGreenConfig(cfg); local != nil {
		return local
	}
	for _, device := range cfg.UGreen {
		if strings.TrimSpace(device.Username) == "" {
			continue
		}
		copied := device
		return &copied
	}
	return nil
}

func buildLocalUGreenConfig(cfg config.AppConfig) *config.UGreenConfig {
	username := strings.TrimSpace(cfg.LocalNasUsername)
	if username == "" {
		return nil
	}

	name := strings.TrimSpace(cfg.LocalNasName)
	if name == "" {
		name = "本机绿联 NAS"
	}

	port := cfg.LocalNasPort
	if port <= 0 {
		port = 9999
	}
	host := strings.TrimSpace(cfg.LocalNasHost)
	if host == "" {
		host = config.DefaultLocalNasHost
	}

	return &config.UGreenConfig{
		ID:             "local-ugreen",
		IpPort:         net.JoinHostPort(host, strconv.Itoa(port)),
		Username:       username,
		Password:       cfg.LocalNasPassword,
		NotifyTypeName: name,
		UseSSL:         false,
	}
}

func ugreenDeviceLabel(cfg config.UGreenConfig) string {
	if name := strings.TrimSpace(cfg.NotifyTypeName); name != "" {
		return name
	}
	if address := strings.TrimSpace(cfg.IpPort); address != "" {
		return address
	}
	return "绿联 NAS"
}

func parseUGreenPerfCommand(command string) (action, mode string, ok bool) {
	command = strings.TrimSpace(command)
	command = strings.Trim(command, "`~'\"“”‘’。?!！？：；;,，")
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", "", false
	}

	rawAction := fields[0]
	upperAction := strings.ToUpper(rawAction)

	switch {
	case strings.HasPrefix(rawAction, "风扇"):
		action = "FAN"
		mode = strings.TrimSpace(strings.TrimPrefix(rawAction, "风扇"))
	case strings.HasPrefix(upperAction, "FAN"):
		action = "FAN"
		mode = strings.TrimSpace(rawAction[len(rawAction)-len(strings.TrimPrefix(upperAction, "FAN")):])
		if strings.EqualFold(rawAction, "fan") {
			mode = ""
		}
		if strings.HasPrefix(upperAction, "FAN") && len(rawAction) > 3 {
			mode = strings.TrimSpace(rawAction[3:])
		}
	case strings.HasPrefix(upperAction, "CPU"):
		action = "CPU"
		mode = strings.TrimSpace(rawAction[3:])
	default:
		return "", "", false
	}

	if mode == "" {
		if len(fields) < 2 {
			return "", "", false
		}
		mode = strings.TrimSpace(fields[1])
	}

	if _, err := strconv.Atoi(mode); err != nil {
		return "", "", false
	}
	return action, mode, true
}

func IsUGreenPerfCommand(command string) bool {
	_, _, ok := parseUGreenPerfCommand(command)
	return ok
}

func ProcessUGreen() {
	cfg := configuredUGreenDevice()
	if cfg == nil {
		return
	}

	ip, port := utils.SplitIpPort(cfg.IpPort, 9999)
	log.Printf("[ugreen] checking device %s (%s:%d, HTTPS=%t)\n", ugreenDeviceLabel(*cfg), ip, port, cfg.UseSSL)
	if !utils.HandleDeviceStatus("绿联", cfg.NotifyTypeName, ip, port) {
		return
	}

	logFile := utils.DeviceLogFile("ugreen", cfg.ID, ip, port)
	authInfo := ensureAuth(cfg.Username, cfg.Password, ip, port, cfg.UseSSL)
	if authInfo == nil {
		log.Printf("[ugreen] login failed for %s\n", ugreenDeviceLabel(*cfg))
		return
	}

	notices, code, err := fetchUGreenNotices(authInfo, ip, port, cfg.UseSSL)
	if err == nil && code != 200 {
		authInfo, err = loginUGreen(cfg.Username, cfg.Password, ip, port, cfg.UseSSL)
		if err == nil {
			notices, code, err = fetchUGreenNotices(authInfo, ip, port, cfg.UseSSL)
		}
	}
	if err != nil {
		log.Printf("[ugreen] fetch notices failed for %s: %v\n", ugreenDeviceLabel(*cfg), err)
		return
	}
	if code != 200 {
		log.Printf("[ugreen] fetch notices failed for %s: API code %d\n", ugreenDeviceLabel(*cfg), code)
		return
	}

	lastTime := getLastUGreenTime(logFile)
	var newNotices []UGreenNotice
	for _, notice := range notices {
		if notice.Time > lastTime {
			newNotices = append(newNotices, notice)
		}
	}

	fileInfo, err := os.Stat(logFile)
	isFirstRun := false
	if err != nil {
		isFirstRun = os.IsNotExist(err)
	} else {
		isFirstRun = fileInfo.Size() == 0
	}

	if isFirstRun || len(newNotices) > 0 {
		if err := saveUGreenNotices(newNotices, logFile); err != nil {
			log.Printf("[ugreen] save notice log failed for %s: %v\n", ugreenDeviceLabel(*cfg), err)
			return
		}
		pushContent := buildUGreenPushContent(newNotices, cfg.NotifyTypeName)
		if pushContent != "" {
			notify.WechatPushCard(buildUGreenNoticeCard(newNotices, cfg.NotifyTypeName), pushContent)
		}
	}

	if len(newNotices) == 0 {
		log.Printf("[ugreen] no new notices for %s\n", ugreenDeviceLabel(*cfg))
	}
}

func PushUGreenSystemStatus() {
	if cfg := configuredUGreenDevice(); cfg != nil {
		pushUGreenSystemStatus(*cfg)
	}
}

func PushUGreenStorageStatus() {
	if cfg := configuredUGreenDevice(); cfg != nil {
		pushUGreenStorageStatus(*cfg)
	}
}

func PushUGreenUpsStatus() {
	if cfg := configuredUGreenDevice(); cfg != nil {
		pushUGreenUpsStatus(*cfg)
	}
}

func PushUGreenNotifyStatus() {
	if cfg := configuredUGreenDevice(); cfg != nil {
		pushUGreenNotifyStatus(*cfg)
	}
}

func PushUGreenDockerStatus() {
	if cfg := configuredUGreenDevice(); cfg != nil {
		pushUGreenDockerStatus(*cfg)
	}
}

func PushUGreenPsStatus() {
	if cfg := configuredUGreenDevice(); cfg != nil {
		pushUGreenPsStatus(*cfg)
	}
}

func PushUGreenBackupStatus() {
	if cfg := configuredUGreenDevice(); cfg != nil {
		pushUGreenBackupStatus(*cfg)
	}
}

func PushUGreenPowerStatus() {
	if cfg := configuredUGreenDevice(); cfg != nil {
		pushUGreenPowerStatus(*cfg)
	}
}

type ugreenPerfRequest struct {
	Method string
	Path   string
	Params map[string]string
	Body   map[string]interface{}
}

func HandleUGreenPerfCommand(command string) {
	action, mode, ok := parseUGreenPerfCommand(command)
	if !ok {
		return
	}

	cfg := configuredUGreenDevice()
	if cfg == nil {
		notify.WechatPush("未配置本机绿联 NAS 管理账号，无法执行性能模式切换。")
		return
	}

	ip, port := utils.SplitIpPort(cfg.IpPort, 9999)
	authInfo, err := ensureAuthWithError(cfg.Username, cfg.Password, ip, port, cfg.UseSSL)
	if err != nil || authInfo == nil {
		notify.WechatPush("登录绿联 NAS 失败，无法执行控制命令。\n" + formatUGreenAuthError(err))
		return
	}

	modeValue, convErr := strconv.Atoi(mode)
	if convErr != nil {
		notify.WechatPush("控制命令参数错误: " + convErr.Error())
		return
	}

	requests, successMessage, buildErr := buildUGreenPerfRequests(action, modeValue, ugreenDeviceLabel(*cfg))
	if buildErr != nil {
		notify.WechatPush("控制命令格式错误: " + buildErr.Error())
		return
	}

	var failures []string
	for _, req := range requests {
		if _, err := requestUGreenDeepAPI(authInfo, ip, port, cfg.UseSSL, req.Method, req.Path, req.Params, req.Body); err == nil {
			notify.WechatPush(successMessage)
			return
		} else {
			failures = append(failures, fmt.Sprintf("%s %s: %v", req.Method, req.Path, err))
		}
	}

	notify.WechatPush("控制命令执行失败: " + strings.Join(failures, " | "))
}

func buildUGreenPerfRequests(action string, mode int, deviceLabel string) ([]ugreenPerfRequest, string, error) {
	switch action {
	case "FAN":
		if mode < 1 || mode > 3 {
			return nil, "", fmt.Errorf("风扇档位必须在 1 到 3 之间")
		}
		modeName := map[int]string{
			1: "静音",
			2: "标准",
			3: "全速",
		}
		modeStr := strconv.Itoa(mode)
		return []ugreenPerfRequest{
				{
					Method: "GET",
					Path:   "/ugreen/v1/hardware/fan/start",
					Params: map[string]string{"mode": modeStr},
				},
				{
					Method: "POST",
					Path:   "/ugreen/v1/taskmgr/power/fan",
					Params: map[string]string{"mode": modeStr},
				},
			},
			fmt.Sprintf("已将 %s 风扇切换为%s模式。", deviceLabel, modeName[mode]),
			nil
	case "CPU":
		if mode < 0 || mode > 2 {
			return nil, "", fmt.Errorf("CPU 模式必须在 0 到 2 之间")
		}
		modeName := map[int]string{
			0: "高性能",
			1: "均衡",
			2: "节能",
		}
		modeStr := strconv.Itoa(mode)
		return []ugreenPerfRequest{
				{
					Method: "POST",
					Path:   "/ugreen/v1/hardware/cpu/frequency",
					Body:   map[string]interface{}{"frequency": mode},
				},
				{
					Method: "POST",
					Path:   "/ugreen/v1/taskmgr/power/cpu",
					Params: map[string]string{"mode": modeStr},
				},
			},
			fmt.Sprintf("已将 %s CPU 切换为%s模式。", deviceLabel, modeName[mode]),
			nil
	default:
		return nil, "", fmt.Errorf("不支持的性能控制命令：%q", action)
	}
}

func pushUGreenSystemStatus(cfg config.UGreenConfig) {
	ip, port := utils.SplitIpPort(cfg.IpPort, 9999)
	authInfo := ensureAuth(cfg.Username, cfg.Password, ip, port, cfg.UseSSL)
	if authInfo == nil {
		log.Printf("[ugreen] fetch system overview failed for %s: login failed\n", ugreenDeviceLabel(cfg))
		return
	}

	info, err := fetchUGreenSystemInfo(authInfo, ip, port, cfg.UseSSL)
	if err != nil {
		authInfo = refreshUGreenAuth(cfg.Username, cfg.Password, ip, port, cfg.UseSSL)
		if authInfo != nil {
			info, err = fetchUGreenSystemInfo(authInfo, ip, port, cfg.UseSSL)
		}
	}
	if err != nil || info == nil {
		log.Printf("[ugreen] fetch system overview failed for %s: %v\n", ugreenDeviceLabel(cfg), err)
		return
	}

	pushContent := buildUGreenSystemStatusPushContent(info, cfg.NotifyTypeName)
	if pushContent != "" {
		notify.WechatPushCard(buildUGreenSystemStatusCard(info, cfg.NotifyTypeName), pushContent)
	}
}

func pushUGreenStorageStatus(cfg config.UGreenConfig) {
	raw, err := requestUGreenAPIWithRetry(cfg, "GET", "/ugreen/v1/storage/volume/list", nil, nil)
	if err != nil {
		notify.WechatPush("获取存储卷信息失败: " + err.Error())
		return
	}

	type volumeItem struct {
		Name       string `json:"name"`
		Label      string `json:"label"`
		PoolName   string `json:"poolname"`
		Total      int64  `json:"total"`
		Used       int64  `json:"used"`
		Status     int    `json:"status"`
		FileSystem string `json:"filesystem"`
	}

	var volumes []volumeItem
	if err := json.Unmarshal(raw, &volumes); err != nil {
		var wrapped struct {
			List   []volumeItem `json:"list"`
			Result []volumeItem `json:"result"`
			Data   []volumeItem `json:"data"`
		}
		if err2 := json.Unmarshal(raw, &wrapped); err2 == nil {
			switch {
			case len(wrapped.Result) > 0:
				volumes = wrapped.Result
			case len(wrapped.List) > 0:
				volumes = wrapped.List
			default:
				volumes = wrapped.Data
			}
		}
	}

	if len(volumes) == 0 {
		notify.WechatPush("当前未获取到存储卷信息。")
		return
	}

	cardVolumes := make([]ugreenStorageCardVolume, 0, len(volumes))
	var builder strings.Builder
	builder.WriteString(wechatCardHeader("STO", "存储卷状态", cfg.NotifyTypeName))
	for i, v := range volumes {
		usagePct := 0.0
		if v.Total > 0 {
			usagePct = float64(v.Used) / float64(v.Total) * 100
		}

		label := fallbackText(v.Label, v.Name)
		label = fallbackText(label, "未命名存储卷")
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(fmt.Sprintf("卷名  %s\n", label))
		if v.PoolName != "" {
			builder.WriteString(fmt.Sprintf("存储池 %s\n", v.PoolName))
		}
		builder.WriteString(wechatPercentLine("已用", usagePct) + "\n")
		builder.WriteString(fmt.Sprintf("容量   %s / %s\n", formatBytesHuman(v.Used), formatBytesHuman(v.Total)))
		if v.FileSystem != "" {
			builder.WriteString(fmt.Sprintf("格式   %s\n", v.FileSystem))
		}
		cardVolumes = append(cardVolumes, ugreenStorageCardVolume{
			Label:      label,
			PoolName:   strings.TrimSpace(v.PoolName),
			FileSystem: strings.TrimSpace(v.FileSystem),
			Total:      v.Total,
			Used:       v.Used,
		})
	}

	notify.WechatPushCard(buildUGreenStorageCard(cardVolumes, cfg.NotifyTypeName), strings.TrimSpace(builder.String()))
}

func pushUGreenUpsStatus(cfg config.UGreenConfig) {
	cfgRaw, err := requestUGreenAPIWithRetry(cfg, "GET", "/ugreen/v1/hardware/ups/config", nil, nil)
	if err != nil {
		notify.WechatPush("获取 UPS 配置失败: " + err.Error())
		return
	}

	usbRaw, usbErr := requestUGreenAPIWithRetry(cfg, "GET", "/ugreen/v1/hardware/ups/usb/info", nil, nil)

	type upsInfoData struct {
		Supplier           string `json:"supplier"`
		ProductMode        string `json:"product_mode"`
		BatteryCapacity    string `json:"battery_capacity"`
		EstimateSupplyTime int    `json:"estimate_supply_time"`
	}

	type upsCfgData struct {
		Status           bool        `json:"status"`
		StandbyTime      int         `json:"standby_time"`
		StandbyTimeUnit  int         `json:"standby_time_unit"`
		ProtectType      int         `json:"protect_type"`
		SnmpUpsConnected bool        `json:"snmp_ups_connected"`
		UpsInfo          upsInfoData `json:"ups_info"`
	}

	var cfgData upsCfgData
	if err := json.Unmarshal(cfgRaw, &cfgData); err != nil {
		var wrapped struct {
			Data upsCfgData `json:"data"`
		}
		if err2 := json.Unmarshal(cfgRaw, &wrapped); err2 == nil {
			cfgData = wrapped.Data
		}
	}

	type usbData struct {
		Supplier     string `json:"supplier"`
		ProductMode  string `json:"product_mode"`
		UsbUpsInsert bool   `json:"usb_ups_insert"`
	}

	var usb usbData
	if usbErr == nil {
		if err := json.Unmarshal(usbRaw, &usb); err != nil {
			var wrapped struct {
				Data usbData `json:"data"`
			}
			if err2 := json.Unmarshal(usbRaw, &wrapped); err2 == nil {
				usb = wrapped.Data
			}
		}
	}

	var builder strings.Builder
	builder.WriteString(wechatCardHeader("UPS", "UPS 供电状态", cfg.NotifyTypeName))
	cardState := ugreenUPSCardState{}

	switch {
	case usb.UsbUpsInsert:
		builder.WriteString(fmt.Sprintf("设备   %s %s（USB）\n", strings.TrimSpace(usb.Supplier), strings.TrimSpace(usb.ProductMode)))
		cardState.Connected = true
		cardState.ConnectionType = "USB"
		cardState.Supplier = strings.TrimSpace(usb.Supplier)
		cardState.ProductModel = strings.TrimSpace(usb.ProductMode)
	case cfgData.SnmpUpsConnected:
		builder.WriteString(fmt.Sprintf("设备   %s %s（SNMP）\n", strings.TrimSpace(cfgData.UpsInfo.Supplier), strings.TrimSpace(cfgData.UpsInfo.ProductMode)))
		cardState.Connected = true
		cardState.ConnectionType = "SNMP"
		cardState.Supplier = strings.TrimSpace(cfgData.UpsInfo.Supplier)
		cardState.ProductModel = strings.TrimSpace(cfgData.UpsInfo.ProductMode)
	default:
		builder.WriteString("未检测到已连接的 UPS 设备。")
		cardState.StatusText = "未连接"
		notify.WechatPushCard(buildUGreenUPSCard(cardState, cfg.NotifyTypeName), strings.TrimSpace(builder.String()))
		return
	}

	if cfgData.Status {
		builder.WriteString("状态   运行中\n")
		cardState.StatusText = "运行中"
	} else {
		builder.WriteString("状态   待机\n")
		cardState.StatusText = "待机"
	}

	capText := strings.TrimSpace(cfgData.UpsInfo.BatteryCapacity)
	if capText == "" {
		builder.WriteString("电量   未知\n")
		cardState.BatteryText = "未知"
	} else {
		displayText := capText
		if normalized, pct, ok := normalizeBatteryPercent(capText); ok {
			displayText = normalized
			cardState.BatteryText = normalized
			cardState.BatteryPercent = pct
			cardState.HasBatteryPct = true
			builder.WriteString(fmt.Sprintf("电量   %s\n", normalized))
			builder.WriteString(wechatPercentLine("电量", pct) + "\n")
		} else {
			if !strings.HasSuffix(displayText, "%") {
				displayText += "%"
			}
			builder.WriteString(fmt.Sprintf("电量   %s\n", displayText))
			cardState.BatteryText = displayText
		}
	}

	est := cfgData.UpsInfo.EstimateSupplyTime
	switch {
	case est < 0:
		builder.WriteString("输入   市电供电中\n")
		cardState.InputStatus = "市电供电中"
		cardState.EnduranceText = "市电供电中"
	case est == 0:
		builder.WriteString("续航   计算中\n")
		cardState.InputStatus = "续航计算中"
		cardState.EnduranceText = "计算中"
	default:
		builder.WriteString(fmt.Sprintf("续航   %d 秒（约 %.1f 分钟）\n", est, float64(est)/60))
		cardState.InputStatus = "电池供电中"
		cardState.EnduranceText = fmt.Sprintf("%.1f 分钟", float64(est)/60)
	}

	protectType := "未知"
	switch cfgData.ProtectType {
	case 0:
		protectType = "无动作"
	case 1:
		protectType = "安全关机"
	case 2:
		protectType = "进入待机"
	}
	builder.WriteString(fmt.Sprintf("策略   %s\n", protectType))
	cardState.ProtectPolicy = protectType

	if cfgData.StandbyTime > 0 {
		unit := "秒"
		if cfgData.StandbyTimeUnit == 1 {
			unit = "分钟"
		}
		builder.WriteString(fmt.Sprintf("延时   %d %s\n", cfgData.StandbyTime, unit))
		cardState.DelayText = fmt.Sprintf("%d %s", cfgData.StandbyTime, unit)
	}

	notify.WechatPushCard(buildUGreenUPSCard(cardState, cfg.NotifyTypeName), strings.TrimSpace(builder.String()))
}

func pushUGreenNotifyStatus(cfg config.UGreenConfig) {
	ip, port := utils.SplitIpPort(cfg.IpPort, 9999)
	authInfo := ensureAuth(cfg.Username, cfg.Password, ip, port, cfg.UseSSL)
	if authInfo == nil {
		log.Printf("[ugreen] fetch notices failed for %s: login failed\n", ugreenDeviceLabel(cfg))
		return
	}

	notices, code, err := fetchUGreenNotices(authInfo, ip, port, cfg.UseSSL)
	if err != nil || code != 200 {
		log.Printf("[ugreen] fetch notices failed for %s: code=%d err=%v\n", ugreenDeviceLabel(cfg), code, err)
		return
	}

	if len(notices) > 0 {
		pushContent := buildUGreenPushContent(notices, cfg.NotifyTypeName+" 近期通知")
		notify.WechatPushCard(buildUGreenNoticeCard(notices, cfg.NotifyTypeName+" 近期通知"), pushContent)
		return
	}
	notify.WechatPush(fmt.Sprintf("%s 当前没有新的系统通知。", cfg.NotifyTypeName))
}

func pushUGreenDockerStatus(cfg config.UGreenConfig) {
	ovRaw, err := requestUGreenAPIWithRetry(cfg, "GET", "/ugreen/v1/docker/view/ObtainOverviewInfo", nil, nil)
	if err != nil {
		notify.WechatPush("获取 Docker 状态失败: " + err.Error())
		return
	}

	var overview struct {
		RunContainerCount int     `json:"runContainerCount"`
		ContainerCount    int     `json:"containerCount"`
		ImageCount        int     `json:"imageCount"`
		CpuUsed           float64 `json:"cpuUsed"`
	}
	_ = json.Unmarshal(ovRaw, &overview)

	listRaw, listErr := requestUGreenAPIWithRetry(
		cfg,
		"POST",
		"/ugreen/v1/docker/container/ContainerListV2",
		nil,
		map[string]interface{}{"pageNum": 1, "pageSize": 200},
	)

	type dockerContainer struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	var containers []dockerContainer
	if listErr == nil {
		var list struct {
			Result []dockerContainer `json:"result"`
			List   []dockerContainer `json:"list"`
		}
		if err := json.Unmarshal(listRaw, &list); err == nil {
			if len(list.Result) > 0 {
				containers = list.Result
			} else {
				containers = list.List
			}
		}
	}

	var builder strings.Builder
	builder.WriteString(wechatCardHeader("DOC", "Docker 运行概览", cfg.NotifyTypeName))
	builder.WriteString(wechatCountLine("运行", overview.RunContainerCount, overview.ContainerCount) + "\n")
	builder.WriteString(wechatPercentLine("负载", overview.CpuUsed) + "\n")
	builder.WriteString(fmt.Sprintf("镜像数 %d\n", overview.ImageCount))

	builder.WriteString(wechatSection("运行中容器"))
	count := 0
	runningNames := make([]string, 0, overview.RunContainerCount)
	for _, c := range containers {
		status := strings.ToLower(strings.TrimSpace(c.Status))
		if status == "running" || strings.HasPrefix(status, "up") {
			runningNames = append(runningNames, trimDisplayText(c.Name, 40))
			if count < 10 {
				builder.WriteString(fmt.Sprintf("%2d. %s\n", count+1, trimDisplayText(c.Name, 26)))
				count++
			}
		}
	}
	if count == 0 {
		builder.WriteString("当前无运行中的容器\n")
	} else if overview.RunContainerCount > count {
		builder.WriteString(fmt.Sprintf("...另有 %d 个运行中容器\n", overview.RunContainerCount-count))
	}

	notify.WechatPushCard(buildUGreenDockerCard(ugreenDockerCardState{
		RunningCount: overview.RunContainerCount,
		TotalCount:   overview.ContainerCount,
		ImageCount:   overview.ImageCount,
		CPUUsed:      overview.CpuUsed,
		RunningNames: runningNames,
	}, cfg.NotifyTypeName), strings.TrimSpace(builder.String()))
}

func pushUGreenPsStatus(cfg config.UGreenConfig) {
	raw, err := requestUGreenAPIWithRetry(cfg, "GET", "/ugreen/v1/taskmgr/services/processes", nil, nil)
	if err != nil {
		notify.WechatPush("获取进程列表失败: " + err.Error())
		return
	}

	type processItem struct {
		Name    string `json:"name"`
		Consume struct {
			CPU    float64 `json:"cpu_used_percent"`
			Memory float64 `json:"mem_used_percent"`
		} `json:"consume"`
	}

	var resp struct {
		Services struct {
			List []processItem `json:"list"`
		} `json:"services"`
		Processes struct {
			List []processItem `json:"list"`
		} `json:"processes"`
	}
	_ = json.Unmarshal(raw, &resp)

	allProcs := append(resp.Services.List, resp.Processes.List...)
	sort.SliceStable(allProcs, func(i, j int) bool {
		if allProcs[i].Consume.CPU == allProcs[j].Consume.CPU {
			return allProcs[i].Consume.Memory > allProcs[j].Consume.Memory
		}
		return allProcs[i].Consume.CPU > allProcs[j].Consume.CPU
	})

	var builder strings.Builder
	builder.WriteString(wechatCardHeader("PS", "进程占用 TOP 5", cfg.NotifyTypeName))
	if len(allProcs) == 0 {
		builder.WriteString("当前未获取到进程数据。")
		notify.WechatPushCard(buildUGreenProcessCard(nil, cfg.NotifyTypeName), strings.TrimSpace(builder.String()))
		return
	}

	cardItems := make([]ugreenProcessCardItem, 0, len(allProcs))
	for _, p := range allProcs {
		cardItems = append(cardItems, ugreenProcessCardItem{
			Name:   strings.TrimSpace(p.Name),
			CPU:    clampPercent(p.Consume.CPU),
			Memory: clampPercent(p.Consume.Memory),
		})
	}

	for i, p := range allProcs {
		if i >= 5 {
			break
		}
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, trimDisplayText(p.Name, 24)))
		builder.WriteString(fmt.Sprintf("   CPU %5.1f%% [%s]\n", clampPercent(p.Consume.CPU), wechatProgressBar(p.Consume.CPU, 10)))
		builder.WriteString(fmt.Sprintf("   内存 %5.1f%% [%s]\n", clampPercent(p.Consume.Memory), wechatProgressBar(p.Consume.Memory, 10)))
	}

	notify.WechatPushCard(buildUGreenProcessCard(cardItems, cfg.NotifyTypeName), strings.TrimSpace(builder.String()))
}

func pushUGreenBackupStatus(cfg config.UGreenConfig) {
	type backupTask struct {
		TaskName     string `json:"task_name"`
		Status       int    `json:"status"`
		LastSyncTime int64  `json:"last_sync_time"`
	}

	var (
		raw []byte
		err error
	)
	candidates := []struct {
		method string
		path   string
		params map[string]string
		body   map[string]interface{}
	}{
		{
			method: "GET",
			path:   "/ugreen/v2/web/syncbackup/task/list",
			params: map[string]string{"backup_type": "backup", "page": "1", "size": "100"},
		},
		{
			method: "GET",
			path:   "/ugreen/v1/web/syncbackup/task/list",
			params: map[string]string{"backup_type": "backup", "page": "1", "size": "100"},
		},
	}
	for _, candidate := range candidates {
		raw, err = requestUGreenAPIWithRetry(cfg, candidate.method, candidate.path, candidate.params, candidate.body)
		if err == nil {
			break
		}
	}
	if err != nil {
		notify.WechatPush("获取备份任务失败: " + err.Error())
		return
	}

	var result struct {
		List   []backupTask `json:"list"`
		Result []backupTask `json:"result"`
	}
	_ = json.Unmarshal(raw, &result)

	tasks := result.List
	if len(tasks) == 0 {
		tasks = result.Result
	}

	var builder strings.Builder
	builder.WriteString(wechatCardHeader("BAK", "备份任务状态", cfg.NotifyTypeName))
	if len(tasks) == 0 {
		builder.WriteString("当前没有配置备份任务。")
		notify.WechatPushCard(buildUGreenBackupCard(nil, cfg.NotifyTypeName), strings.TrimSpace(builder.String()))
		return
	}

	builder.WriteString(fmt.Sprintf("任务数 %d\n", len(tasks)))
	cardItems := make([]ugreenBackupCardItem, 0, len(tasks))
	for i, t := range tasks {
		statusText := "未知"
		switch t.Status {
		case 0:
			statusText = "已停止"
		case 1:
			statusText = "正常"
		case 2:
			statusText = "运行中"
		case 3:
			statusText = "已暂停"
		case 4:
			statusText = "异常"
		}

		lastSync := "从未运行"
		if t.LastSyncTime > 0 {
			lastSync = time.Unix(t.LastSyncTime, 0).Format("2006-01-02 15:04")
		}
		cardItems = append(cardItems, ugreenBackupCardItem{
			Name:     strings.TrimSpace(t.TaskName),
			Status:   statusText,
			LastSync: lastSync,
		})

		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, trimDisplayText(t.TaskName, 28)))
		builder.WriteString(fmt.Sprintf("   状态 %s\n", statusText))
		builder.WriteString(fmt.Sprintf("   同步 %s\n", lastSync))
	}

	notify.WechatPushCard(buildUGreenBackupCard(cardItems, cfg.NotifyTypeName), strings.TrimSpace(builder.String()))
}

func pushUGreenPowerStatus(cfg config.UGreenConfig) {
	raw, err := requestUGreenAPIWithRetry(cfg, "GET", "/ugreen/v1/hardware/power/config", nil, nil)
	if err != nil {
		notify.WechatPush("获取电源配置失败: " + err.Error())
		return
	}

	type powerConfig struct {
		PowerBoot     bool   `json:"power_boot"`
		WakeOn        bool   `json:"wake_on"`
		HardDriveFlag bool   `json:"hard_drive_flag"`
		HardDriveTime int    `json:"hard_drive_time"`
		HardDriveUnit string `json:"hard_drive_unit"`
	}

	var cfgData powerConfig
	if err := json.Unmarshal(raw, &cfgData); err != nil {
		var wrapped struct {
			Data powerConfig `json:"data"`
		}
		if err2 := json.Unmarshal(raw, &wrapped); err2 == nil {
			cfgData = wrapped.Data
		}
	}

	var builder strings.Builder
	builder.WriteString(wechatCardHeader("PWR", "电源与休眠配置", cfg.NotifyTypeName))
	builder.WriteString(fmt.Sprintf("来电开机 %s\n", enabledStatus(cfgData.PowerBoot)))
	builder.WriteString(fmt.Sprintf("网络唤醒 %s\n", enabledStatus(cfgData.WakeOn)))
	builder.WriteString(fmt.Sprintf("磁盘休眠 %s\n", enabledStatus(cfgData.HardDriveFlag)))
	cardState := ugreenPowerCardState{
		PowerBoot:      cfgData.PowerBoot,
		WakeOn:         cfgData.WakeOn,
		HardDriveSleep: cfgData.HardDriveFlag,
	}
	if cfgData.HardDriveFlag {
		unit := "分钟"
		if strings.EqualFold(cfgData.HardDriveUnit, "H") {
			unit = "小时"
		}
		builder.WriteString(fmt.Sprintf("休眠时间 %d %s\n", cfgData.HardDriveTime, unit))
		cardState.HardDriveAfter = fmt.Sprintf("%d %s", cfgData.HardDriveTime, unit)
	}

	notify.WechatPushCard(buildUGreenPowerCard(cardState, cfg.NotifyTypeName), strings.TrimSpace(builder.String()))
}

func requestUGreenAPIWithRetry(cfg config.UGreenConfig, method, path string, params map[string]string, body map[string]interface{}) ([]byte, error) {
	ip, port := utils.SplitIpPort(cfg.IpPort, 9999)
	authInfo := ensureAuth(cfg.Username, cfg.Password, ip, port, cfg.UseSSL)
	if authInfo == nil {
		return nil, fmt.Errorf("登录失败")
	}

	raw, err := requestUGreenDeepAPI(authInfo, ip, port, cfg.UseSSL, method, path, params, body)
	if err == nil {
		return raw, nil
	}

	authInfo = refreshUGreenAuth(cfg.Username, cfg.Password, ip, port, cfg.UseSSL)
	if authInfo == nil {
		return nil, err
	}

	return requestUGreenDeepAPI(authInfo, ip, port, cfg.UseSSL, method, path, params, body)
}
