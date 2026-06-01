package notify

import "nasnotify-go/internal/notifycard"

func PushTestCard() error {
	return WechatPushCard(
		notifycard.Card{
			Title:   "测试通知",
			Device:  "NasNotify",
			Summary: "这是一条用于验证图片卡片推送链路的测试消息，发图失败时会自动回退为原有文字通知。",
			Badges:  []string{"图片卡片", "自动回退", "ClawBot"},
			Metrics: []notifycard.Metric{
				{Label: "发送方式", Value: "图片卡片", Hint: "优先在聊天内直接展示", Tone: notifycard.ToneGood},
				{Label: "回退策略", Value: "已启用", Hint: "发图失败后自动转为文字", Tone: notifycard.ToneGood},
			},
			Sections: []notifycard.Section{
				{
					Title: "验证项目",
					Lines: []string{
						"消息应直接显示为图片卡片，而不是一整段纯文本。",
						"如当前通道暂不支持发图，系统会自动改发文字，不会丢消息。",
					},
				},
			},
		},
		"测试通知\n\n这是一条来自 NasNotify 的测试消息。",
	)
}
