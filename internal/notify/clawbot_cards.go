package notify

import (
	"fmt"

	"nasnotify-go/internal/notifycard"
)

func ClawBotPushMenuCard(note string) error {
	return ClawBotPushCard(ClawBotMenuCard(note), ClawBotMenuText())
}

func ClawBotPushQueryDeckCard() error {
	return ClawBotPushCard(ClawBotQueryDeckCard(), ClawBotQueryDeckText())
}

func ClawBotPushControlDeckCard() error {
	return ClawBotPushCard(ClawBotControlDeckCard(), ClawBotControlDeckText())
}

func ClawBotMenuCard(note string) notifycard.Card {
	entryCount := 3
	queryCount := 10
	controlCount := 7

	card := notifycard.Card{
		Title:   "命令总览",
		Device:  "微信控制台",
		Summary: "直接回复命令即可完成查询或控制，常用结果会优先以黑金图片卡片返回。",
		Badges: []string{
			"主菜单",
			fmt.Sprintf("%d 组入口", entryCount),
			fmt.Sprintf("%d 项查询", queryCount),
			fmt.Sprintf("%d 项控制", controlCount),
		},
		Metrics: []notifycard.Metric{
			{
				Label: "入口命令",
				Value: fmt.Sprintf("%d 组", entryCount),
				Hint:  "菜单 / 查询菜单 / 控制菜单",
				Tone:  notifycard.ToneGood,
			},
			{
				Label: "查询覆盖",
				Value: fmt.Sprintf("%d 项", queryCount),
				Hint:  "巡检、状态、通知、存储、Docker、UPS 等",
				Tone:  notifycard.ToneGood,
			},
			{
				Label: "控制覆盖",
				Value: fmt.Sprintf("%d 项", controlCount),
				Hint:  "远程唤醒、风扇与 CPU 档位",
				Tone:  notifycard.ToneWarm,
			},
			{
				Label: "别名命令",
				Value: "已支持",
				Hint:  "menu / query / control / fan2 / cpu1",
				Tone:  notifycard.ToneDefault,
			},
		},
		Sections: []notifycard.Section{
			{
				Title: "入口命令",
				Lines: []string{
					"菜单 / help / menu：返回这张总览卡片。",
					"查询菜单 / 查询 / query / query deck：展开完整查询命令表。",
					"控制菜单 / 控制 / control / control deck：展开完整控制命令表。",
				},
			},
			{
				Title: "查询命令",
				Lines: []string{
					"巡检 / 诊断 / health：检查接口连通、登录状态和 WOL 配置。",
					"状态 / system：CPU、内存、风扇、网络与设备概览。",
					"通知 / notice / notify：最近系统通知与异常关键词识别。",
					"存储 / storage：卷容量、总使用率与卷详情。",
					"Docker：容器运行概览与 CPU 负载。",
					"进程 / ps：资源占用靠前的服务与进程。",
					"备份 / backup：备份任务状态与最近同步时间。",
					"电源 / power：来电开机、唤醒与磁盘休眠设置。",
					"UPS：连接方式、电量、续航与保护策略。",
					"测试 / test：校验当前微信推送链路。",
				},
			},
			{
				Title: "控制命令",
				Lines: []string{
					"风扇1 / fan1：静音模式，优先安静。",
					"风扇2 / fan2：标准模式，适合日常运行。",
					"风扇3 / fan3：全速模式，优先散热。",
					"CPU0 / cpu0：高性能模式。",
					"CPU1 / cpu1：均衡模式。",
					"CPU2 / cpu2：节能模式。",
					"唤醒 / wol：向本机绿联 NAS 发送远程唤醒魔术包。",
				},
			},
		},
	}
	if note != "" {
		card.Footer = note
	}
	return card
}

func ClawBotQueryDeckCard() notifycard.Card {
	return notifycard.Card{
		Title:   "查询菜单",
		Device:  "微信控制台",
		Summary: "这一组命令专门用于看状态，回复后会优先返回图片卡片。这里把中文指令和英文别名一起列全。",
		Badges:  []string{"查询", "图片卡片", "含英文别名"},
		Metrics: []notifycard.Metric{
			{
				Label: "常用巡检",
				Value: "10 组",
				Hint:  "系统 / 服务 / 值守",
				Tone:  notifycard.ToneGood,
			},
			{
				Label: "返回形式",
				Value: "图片优先",
				Hint:  "失败时自动回退",
				Tone:  notifycard.ToneWarm,
			},
			{
				Label: "入口补充",
				Value: "3 组",
				Hint:  "菜单 / 查询菜单 / 控制菜单",
				Tone:  notifycard.ToneDefault,
			},
			{
				Label: "常用别名",
				Value: "system",
				Hint:  "notice / storage / ps / power / test",
				Tone:  notifycard.ToneDefault,
			},
		},
		Sections: []notifycard.Section{
			{
				Title: "基础查询",
				Lines: []string{
					"巡检 / 诊断 / health：检查 NAS 地址可达性、账号登录、资源概览和远程唤醒配置。",
					"状态 / system：CPU、内存、风扇、网络速率、设备信息与存储概览。",
					"通知 / notice / notify：最近系统通知流与异常关键词识别。",
					"测试 / test：检查当前微信推送链路是否能直接回卡片。",
				},
			},
			{
				Title: "服务与资源",
				Lines: []string{
					"存储 / storage：卷容量、使用率、文件系统与存储池信息。",
					"Docker：容器运行数、镜像数与正在运行的容器列表。",
					"进程 / ps：CPU / 内存占用前列的服务与进程。",
				},
			},
			{
				Title: "值守场景",
				Lines: []string{
					"备份 / backup：任务运行状态、最近同步时间。",
					"电源 / power：来电开机、网络唤醒、磁盘休眠设置。",
					"UPS：连接方式、电池电量、续航预估与保护策略。",
				},
			},
		},
	}
}

func ClawBotControlDeckCard() notifycard.Card {
	return notifycard.Card{
		Title:   "控制菜单",
		Device:  "微信控制台",
		Summary: "这一组命令用于切换 NAS 的常用性能模式。中文命令和英文简写都可以直接发，执行后会在当前聊天里返回结果。",
		Badges:  []string{"控制", "即时执行", "含简写"},
		Metrics: []notifycard.Metric{
			{
				Label: "风扇模式",
				Value: "3 档",
				Hint:  "静音 / 标准 / 全速",
				Tone:  notifycard.ToneWarm,
			},
			{
				Label: "CPU 模式",
				Value: "3 档",
				Hint:  "高性能 / 均衡 / 节能",
				Tone:  notifycard.ToneGood,
			},
			{
				Label: "远程唤醒",
				Value: "1 项",
				Hint:  "唤醒 / wol",
				Tone:  notifycard.ToneWarm,
			},
			{
				Label: "英文简写",
				Value: "fan / cpu",
				Hint:  "fan1-3 / cpu0-2",
				Tone:  notifycard.ToneDefault,
			},
			{
				Label: "执行方式",
				Value: "直接发送",
				Hint:  "不需要“设置”前缀",
				Tone:  notifycard.ToneDefault,
			},
		},
		Sections: []notifycard.Section{
			{
				Title: "风扇控制",
				Lines: []string{
					"风扇1：优先安静，适合轻负载。",
					"风扇2：均衡方案，适合日常运行。",
					"风扇3：优先散热，适合高负载或高温环境。",
					"英文别名同样可用：fan1 / fan2 / fan3。",
				},
			},
			{
				Title: "CPU 控制",
				Lines: []string{
					"CPU0：高性能，优先速度。",
					"CPU1：均衡，适合大多数场景。",
					"CPU2：节能，优先低功耗与低温。",
					"英文简写同样可用：cpu0 / cpu1 / cpu2。",
				},
			},
			{
				Title: "发送示例",
				Lines: []string{
					"直接回复 唤醒：向后台配置的本机绿联 NAS 发送 WOL 魔术包。",
					"直接回复 风扇2：切换到标准风扇模式。",
					"直接回复 CPU1：切换到均衡性能模式。",
					"直接回复 fan3 / cpu0：使用英文简写执行。",
				},
			},
		},
	}
}
