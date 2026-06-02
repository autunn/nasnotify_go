package notify

import (
	"strings"
	"testing"
)

func TestClawBotMenuTextUsesCardLayout(t *testing.T) {
	menu := ClawBotMenuText()
	for _, want := range []string{
		"NAS 通知中心",
		"────────────────",
		"查询菜单",
		"控制菜单",
		"巡检",
		"UPS",
		"唤醒",
		"风扇2",
		"CPU1",
		"一键巡检  `巡检`",
		"风扇控制  `风扇2`",
		"性能模式  `CPU1`",
	} {
		if !strings.Contains(menu, want) {
			t.Fatalf("menu does not contain %q:\n%s", want, menu)
		}
	}
}

func TestClawBotDeckTexts(t *testing.T) {
	query := ClawBotQueryDeckText()
	for _, want := range []string{"查询菜单", "`巡检`", "`状态`", "`Docker`", "`测试`"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query deck does not contain %q:\n%s", want, query)
		}
	}

	control := ClawBotControlDeckText()
	for _, want := range []string{"控制菜单", "`风扇2`", "`fan2`", "`CPU1`", "`唤醒`", "`wol`", "具体指令"} {
		if !strings.Contains(control, want) {
			t.Fatalf("control deck does not contain %q:\n%s", want, control)
		}
	}
}
