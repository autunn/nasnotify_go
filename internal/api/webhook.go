package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"nasnotify-go/internal/config"
	"nasnotify-go/internal/nas"
	"nasnotify-go/internal/notify"

	"github.com/gin-gonic/gin"
)

// HandleVerify 处理企业微信的 URL 验证及普通 Webhook 的 GET 请求
func HandleVerify(c *gin.Context) {
	echostr := c.Query("echostr")
	if echostr == "" && (c.Query("text") != "" || c.Query("message") != "" || c.Query("task") != "") {
		HandleMessage(c)
		return
	}

	config.CfgMu.RLock()
	token := config.Config.Token
	aesKeyStr := config.Config.EncodingAESKey
	config.CfgMu.RUnlock()

	msgSig := c.Query("msg_signature")
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")

	params := []string{token, timestamp, nonce, echostr}
	sort.Strings(params)

	h := sha1.New()
	h.Write([]byte(strings.Join(params, "")))

	if fmt.Sprintf("%x", h.Sum(nil)) != msgSig {
		c.AbortWithStatus(403)
		return
	}

	aesKey, err := base64.StdEncoding.DecodeString(aesKeyStr + "=")
	if err != nil || len(aesKey) != 32 {
		c.AbortWithStatus(403)
		return
	}

	cipherText, err := base64.StdEncoding.DecodeString(echostr)
	if err != nil || len(cipherText) < 32 || len(cipherText)%aes.BlockSize != 0 {
		c.AbortWithStatus(403)
		return
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		c.AbortWithStatus(403)
		return
	}

	mode := cipher.NewCBCDecrypter(block, aesKey[:16])
	mode.CryptBlocks(cipherText, cipherText)

	msgLen := binary.BigEndian.Uint32(cipherText[16:20])
	if int(msgLen)+20 > len(cipherText) {
		c.AbortWithStatus(403)
		return
	}
	c.String(200, string(cipherText[20:20+msgLen]))
}

// HandleMessage 统一处理接收到的通用 Webhook 推送与企业微信交互事件
func HandleMessage(c *gin.Context) {
	bodyBytes, _ := io.ReadAll(c.Request.Body)

	var xmlMsg notify.WeChatXMLMsg
	if len(bodyBytes) > 0 {
		if err := xml.Unmarshal(bodyBytes, &xmlMsg); err == nil && xmlMsg.Encrypt != "" {
			if !verifyWeChatSignature(c, xmlMsg.Encrypt) {
				c.AbortWithStatus(403)
				return
			}
			processWechatEvent(c, xmlMsg.Encrypt)
			return
		}
	}

	data := make(map[string]interface{})

	for k, v := range c.Request.URL.Query() {
		if len(v) > 0 {
			data[k] = v[0]
		}
	}

	if len(bodyBytes) > 0 {
		var jsonData map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &jsonData); err == nil {
			for k, v := range jsonData {
				data[k] = v
			}
		} else {
			if len(data) == 0 {
				data["raw_message"] = string(bodyBytes)
			}
		}
	}

	if len(data) > 0 {
		var description strings.Builder
		description.WriteString(fmt.Sprintf("外部 Webhook 触发\n触发时间: %s", time.Now().Format("2006-01-02 15:04:05")))

		for k, v := range data {
			description.WriteString(fmt.Sprintf("\n%s: %v", k, v))
		}

		go notify.WechatPush(description.String())
	}

	c.JSON(200, gin.H{"status": "ok"})
}

// processWechatEvent 解密企业微信的指令并执行对应操作
func processWechatEvent(c *gin.Context, encryptStr string) {
	config.CfgMu.RLock()
	aesKeyStr := config.Config.EncodingAESKey
	config.CfgMu.RUnlock()

	aesKey, err := base64.StdEncoding.DecodeString(aesKeyStr + "=")
	if err != nil || len(aesKey) != 32 {
		c.String(200, "success")
		return
	}

	cipherText, err := base64.StdEncoding.DecodeString(encryptStr)
	if err != nil || len(cipherText) < 32 || len(cipherText)%aes.BlockSize != 0 {
		c.String(200, "success")
		return
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		c.String(200, "success")
		return
	}

	mode := cipher.NewCBCDecrypter(block, aesKey[:16])
	mode.CryptBlocks(cipherText, cipherText)

	msgLen := binary.BigEndian.Uint32(cipherText[16:20])
	if int(msgLen)+20 > len(cipherText) {
		c.String(200, "success")
		return
	}
	plainXmlBytes := cipherText[20 : 20+msgLen]

	var plainMsg notify.WeChatPlainMsg
	if err := xml.Unmarshal(plainXmlBytes, &plainMsg); err == nil {
		// ==================== 1. 拦截菜单点击事件 ====================
		if plainMsg.MsgType == "event" && plainMsg.Event == "click" {
			switch plainMsg.EventKey {
			case "GET_UGREEN_HEALTH":
				go nas.PushUGreenHealthCheck()

			case "GET_UGREEN_INFO":
				go nas.PushUGreenSystemStatus()

			case "GET_UGREEN_STORAGE":
				go nas.PushUGreenStorageStatus()

			case "GET_UGREEN_UPS":
				go nas.PushUGreenUpsStatus()

			case "GET_UGREEN_DOCKER":
				go nas.PushUGreenDockerStatus()

			case "GET_UGREEN_PS":
				go nas.PushUGreenPsStatus()

			case "GET_UGREEN_BACKUP":
				go nas.PushUGreenBackupStatus()

			case "GET_UGREEN_POWER":
				go nas.PushUGreenPowerStatus()

			case "GET_UGREEN_NOTIFY":
				go nas.PushUGreenNotifyStatus()

			case "GET_UGREEN_QUERY_HELP":
				go notify.WechatPush("更多查询指令\n\n可直接在企业微信聊天框发送：\n\nDocker：查看容器运行概览\n进程：查看资源占用 TOP 5\n备份：查看备份任务状态\n电源：查看电源与休眠配置\nUPS：查看 UPS 供电状态\n测试：发送测试通知\n\n控制指令可在“控制”菜单直接点击，也可发送：风扇1 / 风扇2 / 风扇3，CPU0 / CPU1 / CPU2。")

			case "GET_UGREEN_PERF":
				go notify.WechatPush("️ **性能控制向导**\n\n请直接在聊天框回复以下指令：\n\n **风扇控制**\n「风扇 1 设备名」: 静音模式\n「风扇 2 设备名」: 正常模式\n「风扇 3 设备名」: 全速模式\n\n⚡ **CPU 模式**\n「CPU 0 设备名」: 高性能\n「CPU 1 设备名」: 均衡\n「CPU 2 设备名」: 节能\n\n如果只配置了一台绿联设备，也可以直接省略设备名。")

			case "GET_UGREEN_CPU_HELP":
				go notify.WechatPush("CPU 性能设置\n\n请直接发送以下指令执行：\n\nCPU0：高性能模式，优先响应速度\nCPU1：均衡模式，适合日常使用\nCPU2：节能模式，优先低功耗与低温\n\n风扇转速可直接点击“控制”菜单里的风扇静音、风扇标准、风扇全速。")

			case "GET_TEST_PUSH":
				go func() {
					if err := notify.PushTestCard(); err != nil {
						notify.WechatPush("测试通知发送失败: " + err.Error())
					}
				}()

			case "GET_NAS_WOL":
				go nas.HandleWakeMenuCommand()

			case "SET_UGREEN_FAN_1":
				go nas.HandleUGreenPerfCommand("风扇1")

			case "SET_UGREEN_FAN_2":
				go nas.HandleUGreenPerfCommand("风扇2")

			case "SET_UGREEN_FAN_3":
				go nas.HandleUGreenPerfCommand("风扇3")

			case "SET_UGREEN_CPU_0":
				go nas.HandleUGreenPerfCommand("CPU0")

			case "SET_UGREEN_CPU_1":
				go nas.HandleUGreenPerfCommand("CPU1")

			case "SET_UGREEN_CPU_2":
				go nas.HandleUGreenPerfCommand("CPU2")
			}
		}

		// ==================== 2. 拦截用户文本输入 ====================
		if plainMsg.MsgType == "text" {
			content := strings.TrimSpace(plainMsg.Content)
			normalized := normalizeWechatCommand(content)

			switch {
			case normalized == "巡检" || normalized == "诊断" || normalized == "health" || normalized == "check":
				go nas.PushUGreenHealthCheck()
			case normalized == "状态" || normalized == "系统" || normalized == "概览" || normalized == "system" || normalized == "status" || normalized == "info":
				go nas.PushUGreenSystemStatus()
			case normalized == "通知" || normalized == "消息" || normalized == "notice" || normalized == "notify" || normalized == "message":
				go nas.PushUGreenNotifyStatus()
			case normalized == "存储" || normalized == "硬盘" || normalized == "磁盘" || normalized == "storage" || normalized == "disk":
				go nas.PushUGreenStorageStatus()
			case normalized == "docker" || normalized == "容器" || normalized == "container":
				go nas.PushUGreenDockerStatus()
			case normalized == "进程" || normalized == "服务" || normalized == "ps" || normalized == "process":
				go nas.PushUGreenPsStatus()
			case normalized == "备份" || normalized == "同步" || normalized == "backup" || normalized == "sync":
				go nas.PushUGreenBackupStatus()
			case normalized == "电源" || normalized == "休眠" || normalized == "power" || normalized == "sleep":
				go nas.PushUGreenPowerStatus()
			case normalized == "ups":
				go nas.PushUGreenUpsStatus()
			case normalized == "测试" || normalized == "test":
				go func() {
					if err := notify.PushTestCard(); err != nil {
						notify.WechatPush("测试通知发送失败: " + err.Error())
					}
				}()
			case strings.HasPrefix(content, "风扇") || strings.HasPrefix(strings.ToUpper(content), "CPU") || strings.HasPrefix(strings.ToUpper(content), "FAN"):
				go nas.HandleUGreenPerfCommand(content)
			case strings.HasPrefix(content, "唤醒") || normalized == "wol" || normalized == "wake" || strings.HasPrefix(normalized, "wol ") || strings.HasPrefix(normalized, "wake "):
				targetName := enterpriseWakeTargetFromCommand(content, normalized)
				go nas.HandleWakeCommand(targetName)
			default:
				go notify.WechatPush("未识别指令。\n\n可发送：巡检、状态、通知、存储、Docker、进程、备份、电源、UPS、测试、唤醒、风扇1、风扇2、风扇3、CPU0、CPU1、CPU2。")
			}
		}
	}

	c.String(200, "success")
}

func verifyWeChatSignature(c *gin.Context, encrypt string) bool {
	config.CfgMu.RLock()
	token := config.Config.Token
	config.CfgMu.RUnlock()

	msgSig := c.Query("msg_signature")
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")
	if token == "" || msgSig == "" || timestamp == "" || nonce == "" {
		return false
	}

	params := []string{token, timestamp, nonce, encrypt}
	sort.Strings(params)

	h := sha1.New()
	h.Write([]byte(strings.Join(params, "")))
	return fmt.Sprintf("%x", h.Sum(nil)) == msgSig
}

func enterpriseWakeTargetFromCommand(command, normalized string) string {
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

func normalizeWechatCommand(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Trim(text, "`~'\"“”‘’。、?!！？：；;，,")
	text = strings.NewReplacer("\u3000", " ", "\t", " ", "\r", " ", "\n", " ").Replace(text)
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}
