package dysms

import (
	"context"
	"errors"
	"fmt"
	"strings"

	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	dysmsapi "github.com/alibabacloud-go/dysmsapi-20170525/v2/client"
	"github.com/alibabacloud-go/tea/tea"

	"github.com/jackmengtag/SonicHub_api/internal/config"
)

// Client 封装阿里云短信服务（Dysmsapi），用于发送自建验证码。
type Client struct {
	enabled  bool
	client   *dysmsapi.Client
	signName string
	template string
	param    string
}

// New 根据配置创建短信客户端；签名或模板缺失时返回禁用状态。
func New(cfg config.Config) (*Client, error) {
	c := &Client{
		signName: cfg.AliyunSMSSignName,
		template: cfg.AliyunSMSTemplateCode,
		param:    cfg.AliyunSMSTemplateParam,
	}
	if strings.TrimSpace(cfg.AliyunAccessKeyID) == "" || strings.TrimSpace(cfg.AliyunAccessKeySecret) == "" ||
		strings.TrimSpace(c.signName) == "" || strings.TrimSpace(c.template) == "" {
		return c, nil
	}
	client, err := dysmsapi.NewClient(&openapi.Config{
		AccessKeyId:     tea.String(cfg.AliyunAccessKeyID),
		AccessKeySecret: tea.String(cfg.AliyunAccessKeySecret),
		Endpoint:        tea.String("dysmsapi.aliyuncs.com"),
	})
	if err != nil {
		return nil, fmt.Errorf("init dysmsapi client: %w", err)
	}
	c.client = client
	c.enabled = true
	return c, nil
}

// Enabled 返回短信服务通道是否可用。
func (c *Client) Enabled() bool { return c != nil && c.enabled }

// Send 发送包含指定验证码的短信，返回阿里云 BizId。
func (c *Client) Send(ctx context.Context, phone, code string) (string, error) {
	if !c.Enabled() {
		return "", errors.New("dysms is not configured")
	}
	param := strings.ReplaceAll(c.param, "##code##", code)
	if param == "" || !strings.Contains(c.param, "##code##") {
		param = fmt.Sprintf(`{"code":%q}`, code)
	}
	resp, err := c.client.SendSms(&dysmsapi.SendSmsRequest{
		PhoneNumbers:  tea.String(phone),
		SignName:      tea.String(c.signName),
		TemplateCode:  tea.String(c.template),
		TemplateParam: tea.String(param),
	})
	if err != nil {
		return "", fmt.Errorf("aliyun dysms send: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return "", errors.New("aliyun dysms send: empty response")
	}
	if result := tea.StringValue(resp.Body.Code); result != "" && result != "OK" {
		return "", errors.New("aliyun dysms send: " + firstNonEmpty(tea.StringValue(resp.Body.Message), result))
	}
	return tea.StringValue(resp.Body.BizId), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
