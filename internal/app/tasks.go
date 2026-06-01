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
	compact := strings.ReplaceAll(normalized, " ", "")
	switch {
	case normalized == "菜单" || normalized == "help" || normalized == "menu":
		notify.ClawBotPushCard(notify.ClawBotMenuCard(""), notify.ClawBotMenuText())
	case normalized == "query deck" || compact == "querydeck" || normalized == "查询菜单" || normalized == "查询":
		notify.ClawBotPushCard(notify.ClawBotQueryDeckCard(), notify.ClawBotQueryDeckText())
	case normalized == "control deck" || compact == "controldeck" || normalized == "控制菜单" || normalized == "控制":
		notify.ClawBotPushCard(notify.ClawBotControlDeckCard(), notify.ClawBotControlDeckText())
	case normalized == "状态" || normalized == "system":
		nas.PushUGreenSystemStatus()
	case normalized == "通知" || normalized == "notice" || normalized == "notify":
		nas.PushUGreenNotifyStatus()
	case normalized == "存储" || normalized == "storage":
		nas.PushUGreenStorageStatus()
	case normalized == "docker":
		nas.PushUGreenDockerStatus()
	case normalized == "进程" || normalized == "ps":
		nas.PushUGreenPsStatus()
	case normalized == "备份" || normalized == "backup":
		nas.PushUGreenBackupStatus()
	case normalized == "电源" || normalized == "power":
		nas.PushUGreenPowerStatus()
	case normalized == "ups":
		nas.PushUGreenUpsStatus()
	case normalized == "测试" || normalized == "test":
		if err := triggerTestPush(); err != nil {
			notify.WechatPush("测试通知发送失败: " + err.Error())
		}
	case nas.IsUGreenPerfCommand(command):
		nas.HandleUGreenPerfCommand(command)
	default:
		notify.ClawBotPushCard(notify.ClawBotMenuCard("未识别命令，下面是当前可用菜单。"), "未识别命令。\n\n"+notify.ClawBotMenuText())
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
