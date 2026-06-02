package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"nasnotify-go/internal/config"
	"nasnotify-go/internal/nas"
	"nasnotify-go/internal/notify"
)

const (
	clawBotInitialPollDelay    = 1 * time.Second
	clawBotCommandPollInterval = 2 * time.Second
)

type TaskRuntime struct{}

func NewTaskRuntime() *TaskRuntime {
	return &TaskRuntime{}
}

func (t *TaskRuntime) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(3)
	go runTasksLoop(ctx, wg)
	go runSystemStatusTasksLoop(ctx, wg)
	go runClawBotCommandLoop(ctx, wg)
}

func runTasksLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	if !sleepWithContext(ctx, 2*time.Second) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		startedAt := time.Now()
		nas.ProcessUGreen()
		if !waitForConfiguredInterval(ctx, startedAt, configuredNotificationIntervalMinutes, config.DefaultIntervalMinutes) {
			return
		}
	}
}

func runSystemStatusTasksLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	if !sleepWithContext(ctx, 3*time.Second) {
		return
	}
	for {
		if !waitForConfiguredInterval(ctx, time.Now(), configuredSystemStatusIntervalMinutes, config.DefaultSystemStatusIntervalMinutes) {
			return
		}
		nas.PushUGreenSystemStatus()
	}
}

var clawBotCommandMu sync.Mutex
var processedClawBotMessages = make(map[string]time.Time)

func triggerTestPush() error {
	return notify.PushTestCard()
}

func runClawBotCommandLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	if !sleepWithContext(ctx, clawBotInitialPollDelay) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		pollClawBotCommandsOnce()
		if !sleepWithContext(ctx, clawBotCommandPollInterval) {
			return
		}
	}
}

func pollClawBotCommandsOnce() {
	if !config.IsInitialized() || !notify.ClawBotConfigured() {
		return
	}
	messages, err := notify.GetClawBotMessages()
	if err != nil || len(messages) == 0 {
		return
	}
	for _, msg := range messages {
		text := strings.TrimSpace(msg.Text)
		if text == "" || notify.MatchClawBotBindingMessage(text) {
			continue
		}
		if !notify.ClawBotBound() || !shouldProcessClawBotCommand(msg) {
			continue
		}
		handleClawBotCommand(text)
	}
}

func shouldProcessClawBotCommand(msg notify.ClawBotMessage) bool {
	clawBotCommandMu.Lock()
	defer clawBotCommandMu.Unlock()
	now := time.Now()
	key := clawBotMessageKey(msg)
	if key == "" {
		return false
	}
	for oldKey, seenAt := range processedClawBotMessages {
		if now.Sub(seenAt) > 10*time.Minute {
			delete(processedClawBotMessages, oldKey)
		}
	}
	if _, exists := processedClawBotMessages[key]; exists {
		return false
	}
	processedClawBotMessages[key] = now
	return true
}

func clawBotMessageKey(msg notify.ClawBotMessage) string {
	if id := strings.TrimSpace(msg.ID); id != "" {
		return "id:" + id
	}
	text := strings.ToLower(strings.TrimSpace(msg.Text))
	if text == "" {
		return ""
	}
	return fmt.Sprintf("text:%s:%s", text, strings.TrimSpace(msg.CreatedAt))
}

func handleClawBotCommand(text string) {
	command := strings.TrimSpace(text)
	normalized := normalizeClawBotCommand(command)
	compact := removeCommandSpaces(normalized)
	switch {
	case commandMatches(normalized, compact, "菜单", "帮助", "help", "menu"):
		notify.ClawBotPushCard(notify.ClawBotMenuCard(""), notify.ClawBotMenuText())
	case commandMatches(normalized, compact, "query deck", "querydeck", "查询菜单", "查询"):
		notify.ClawBotPushCard(notify.ClawBotQueryDeckCard(), notify.ClawBotQueryDeckText())
	case commandMatches(normalized, compact, "control deck", "controldeck", "控制菜单", "控制"):
		notify.ClawBotPushCard(notify.ClawBotControlDeckCard(), notify.ClawBotControlDeckText())
	case commandMatches(normalized, compact, "巡检", "诊断", "health", "check"):
		nas.PushUGreenHealthCheck()
	case commandMatches(normalized, compact, "状态", "概览", "系统", "system", "info", "status"):
		nas.PushUGreenSystemStatus()
	case commandMatches(normalized, compact, "通知", "消息", "notice", "notify", "message"):
		nas.PushUGreenNotifyStatus()
	case commandMatches(normalized, compact, "存储", "硬盘", "磁盘", "storage", "disk"):
		nas.PushUGreenStorageStatus()
	case commandMatches(normalized, compact, "docker", "容器", "container"):
		nas.PushUGreenDockerStatus()
	case commandMatches(normalized, compact, "进程", "服务", "ps", "process"):
		nas.PushUGreenPsStatus()
	case commandMatches(normalized, compact, "备份", "同步", "backup", "sync"):
		nas.PushUGreenBackupStatus()
	case commandMatches(normalized, compact, "电源", "休眠", "power", "sleep"):
		nas.PushUGreenPowerStatus()
	case commandMatches(normalized, compact, "ups"):
		nas.PushUGreenUpsStatus()
	case commandMatches(normalized, compact, "测试", "test"):
		if err := triggerTestPush(); err != nil {
			notify.WechatPush("测试通知发送失败: " + err.Error())
		}
	case isWakeCommand(normalized, compact):
		targetName := wakeTargetFromCommand(command, normalized)
		nas.HandleWakeCommand(targetName)
	case nas.IsUGreenPerfCommand(command):
		nas.HandleUGreenPerfCommand(command)
	default:
		notify.ClawBotPushCard(notify.ClawBotMenuCard("未识别命令，下面是当前可用菜单。"), "未识别命令。\n\n"+notify.ClawBotMenuText())
	}
}

func removeCommandSpaces(text string) string {
	return strings.ReplaceAll(text, " ", "")
}

func commandMatches(normalized, compact string, aliases ...string) bool {
	for _, alias := range aliases {
		aliasNormalized := normalizeClawBotCommand(alias)
		if normalized == aliasNormalized || compact == removeCommandSpaces(aliasNormalized) {
			return true
		}
	}
	return false
}

func isWakeCommand(normalized, compact string) bool {
	return commandMatches(normalized, compact, "唤醒", "wol", "wake") ||
		strings.HasPrefix(normalized, "唤醒 ") ||
		strings.HasPrefix(normalized, "wol ") ||
		strings.HasPrefix(normalized, "wake ")
}

func wakeTargetFromCommand(command, normalized string) string {
	switch {
	case strings.HasPrefix(command, "唤醒"):
		return strings.TrimSpace(strings.TrimPrefix(command, "唤醒"))
	case strings.HasPrefix(normalized, "wol "):
		return strings.TrimSpace(command[3:])
	case strings.HasPrefix(normalized, "wake "):
		return strings.TrimSpace(command[4:])
	default:
		return ""
	}
}

func normalizeClawBotCommand(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Trim(text, "`~'\"“”‘’。、?!！？：；;，,")
	text = strings.NewReplacer("\u3000", " ", "\t", " ", "\r", " ", "\n", " ").Replace(text)
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

func configuredNotificationIntervalMinutes() float64 {
	return config.GetConfigSnapshot().IntervalMinutes
}
func configuredSystemStatusIntervalMinutes() float64 {
	return config.GetConfigSnapshot().SystemStatusIntervalMinutes
}
func durationFromMinutes(minutes, fallback float64) time.Duration {
	if minutes <= 0 {
		minutes = fallback
	}
	return time.Duration(minutes * float64(time.Minute))
}

func DurationFromMinutesForTest(minutes, fallback float64) time.Duration {
	return durationFromMinutes(minutes, fallback)
}

func NormalizeClawBotCommandForTest(text string) string {
	return normalizeClawBotCommand(text)
}

func waitForConfiguredInterval(ctx context.Context, startedAt time.Time, intervalMinutes func() float64, fallbackMinutes float64) bool {
	for {
		duration := durationFromMinutes(intervalMinutes(), fallbackMinutes)
		remaining := duration - time.Since(startedAt)
		if remaining <= 0 {
			return true
		}
		if remaining > 30*time.Second {
			remaining = 30 * time.Second
		}
		if !sleepWithContext(ctx, remaining) {
			return false
		}
	}
}

func sleepWithContext(ctx context.Context, wait time.Duration) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func waitBackgroundLoops(wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}
