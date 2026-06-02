package notify

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"nasnotify-go/internal/notifycard"
)

func WechatPush(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	var errs []string
	sent := false

	if EnterpriseWechatConfigured() {
		if err := EnterpriseWechatPush(content); err != nil {
			errs = append(errs, "企业微信: "+err.Error())
			log.Printf("enterprise wechat push failed: %v", err)
		} else {
			sent = true
		}
	}

	if ClawBotConfigured() && ClawBotBound() {
		if err := ClawBotPush(content); err != nil {
			errs = append(errs, "ClawBot: "+err.Error())
			log.Printf("clawbot push failed: %v", err)
		} else {
			sent = true
		}
	}

	if sent {
		return nil
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, " | "))
	}
	return fmt.Errorf("未配置可用的推送通道")
}

func WechatPushCard(card notifycard.Card, fallbackText string) error {
	card = enrichNotifyCard(card)
	fallbackText = strings.TrimSpace(fallbackText)
	if fallbackText == "" {
		fallbackText = strings.TrimSpace(card.Title)
		if summary := strings.TrimSpace(card.Summary); summary != "" {
			if fallbackText != "" {
				fallbackText += "\n\n"
			}
			fallbackText += summary
		}
	}

	var errs []string
	sent := false

	if EnterpriseWechatConfigured() {
		pngData, err := notifycard.RenderPNG(card)
		if err != nil {
			errs = append(errs, "企业微信图片渲染: "+err.Error())
			log.Printf("render enterprise notify card failed: %v", err)
		} else if err := EnterpriseWechatPushImage(pngData); err != nil {
			errs = append(errs, "企业微信图片: "+err.Error())
			log.Printf("enterprise wechat image push failed: %v", err)
		} else {
			sent = true
		}
	}

	if ClawBotConfigured() && ClawBotBound() {
		if err := ClawBotPushCard(card, fallbackText); err != nil {
			errs = append(errs, "ClawBot图片: "+err.Error())
			log.Printf("clawbot card push failed: %v", err)
		} else {
			sent = true
		}
	}

	if sent {
		return nil
	}
	if fallbackText != "" {
		if err := WechatPush(fallbackText); err != nil {
			if len(errs) > 0 {
				return fmt.Errorf("%s | 回退文本: %w", strings.Join(errs, " | "), err)
			}
			return err
		}
		return nil
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, " | "))
	}
	return fmt.Errorf("未配置可用的图片推送通道")
}
