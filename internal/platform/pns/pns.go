package pns

import (
	"context"
	"errors"
	"fmt"
	"strings"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dypnsapi "github.com/alibabacloud-go/dypnsapi-20170525/v2/client"
	"github.com/alibabacloud-go/tea/tea"

	"github.com/jackmengtag/SonicHub_api/internal/config"
)

// ErrNotConfigured 表示没有配置阿里云号码认证服务参数。
var ErrNotConfigured = errors.New("pns provider is not configured")

// Client 封装阿里云号码认证服务（Dypnsapi）：短信验证码与一键登录取号。
type Client struct {
	enabled  bool
	client   *dypnsapi.Client
	signName string
	template string
	param    string
	scheme   string
	scene    string
}

// New 根据配置创建号码认证客户端；未配置 AccessKey 时返回一个禁用状态的客户端。
func New(cfg config.Config) (*Client, error) {
	c := &Client{
		signName: cfg.AliyunSMSSignName,
		template: cfg.AliyunSMSTemplateCode,
		param:    cfg.AliyunSMSTemplateParam,
		scheme:   cfg.AliyunSMSSchemeName,
		scene:    cfg.AliyunSMSSceneCode,
	}
	if strings.TrimSpace(cfg.AliyunAccessKeyID) == "" || strings.TrimSpace(cfg.AliyunAccessKeySecret) == "" {
		return c, nil
	}
	client, err := dypnsapi.NewClient(&openapi.Config{
		AccessKeyId:     tea.String(cfg.AliyunAccessKeyID),
		AccessKeySecret: tea.String(cfg.AliyunAccessKeySecret),
		Endpoint:        tea.String(cfg.AliyunSMSEndpoint),
	})
	if err != nil {
		return nil, fmt.Errorf("init dypnsapi client: %w", err)
	}
	c.client = client
	c.enabled = true
	return c, nil
}

// Enabled 表示短信验证码通道（SendSmsVerifyCode / CheckSmsVerifyCode）可用。
func (c *Client) Enabled() bool {
	return c.configured() && strings.TrimSpace(c.signName) != "" && strings.TrimSpace(c.template) != ""
}

// configured 表示 AK/SK 已就绪且客户端初始化成功。
func (c *Client) configured() bool { return c != nil && c.enabled }

// Available 表示 AK/SK 已配置，可调用服务端接口（如一键登录、验证码核验）。
func (c *Client) Available() bool { return c.configured() }

// SDKEnabled 表示是否具备 Android 短信验证码 SDK 流程所需参数（方案号 + 模板）。
func (c *Client) SDKEnabled() bool {
	return c.configured() && strings.TrimSpace(c.scene) != "" && strings.TrimSpace(c.template) != ""
}

// Send 下发短信验证码，返回阿里云业务 ID（用于排查发送记录）。
func (c *Client) Send(ctx context.Context, phone string) (string, error) {
	if !c.Enabled() {
		return "", ErrNotConfigured
	}
	req := &dypnsapi.SendSmsVerifyCodeRequest{
		PhoneNumber:      tea.String(phone),
		CountryCode:      tea.String("86"),
		SignName:         tea.String(c.signName),
		TemplateCode:     tea.String(c.template),
		TemplateParam:    tea.String(c.param),
		CodeLength:       tea.Int64(6),
		CodeType:         tea.Int64(1),
		ValidTime:        tea.Int64(300),
		Interval:         tea.Int64(60),
		DuplicatePolicy:  tea.Int64(1),
		ReturnVerifyCode: tea.Bool(false),
	}
	if c.scheme != "" {
		req.SchemeName = tea.String(c.scheme)
	}
	resp, err := c.client.SendSmsVerifyCode(req)
	if err != nil {
		return "", fmt.Errorf("aliyun send sms: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return "", errors.New("aliyun send sms: empty response")
	}
	if code := tea.StringValue(resp.Body.Code); code != "" && code != "OK" {
		return "", errors.New("aliyun send sms: " + firstNonEmpty(tea.StringValue(resp.Body.Message), code))
	}
	bizID := ""
	if resp.Body.Model != nil {
		bizID = tea.StringValue(resp.Body.Model.BizId)
	}
	return bizID, nil
}

// Verify 核验用户提交的短信验证码。
func (c *Client) Verify(ctx context.Context, phone, code string) (bool, error) {
	if !c.Enabled() {
		return false, ErrNotConfigured
	}
	req := &dypnsapi.CheckSmsVerifyCodeRequest{
		PhoneNumber:    tea.String(phone),
		CountryCode:    tea.String("86"),
		VerifyCode:     tea.String(strings.TrimSpace(code)),
		CaseAuthPolicy: tea.Int64(1),
	}
	if c.scheme != "" {
		req.SchemeName = tea.String(c.scheme)
	}
	resp, err := c.client.CheckSmsVerifyCode(req)
	if err != nil {
		// 验证码错误或过期时阿里云返回 isv.ValidateFail，属于正常核验结果。
		if strings.Contains(err.Error(), "isv.ValidateFail") {
			return false, nil
		}
		return false, fmt.Errorf("aliyun check sms: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return false, errors.New("aliyun check sms: empty response")
	}
	if code := tea.StringValue(resp.Body.Code); code != "" && code != "OK" {
		return false, errors.New("aliyun check sms: " + firstNonEmpty(tea.StringValue(resp.Body.Message), code))
	}
	if resp.Body.Model == nil {
		return false, nil
	}
	return tea.StringValue(resp.Body.Model.VerifyResult) == "PASS", nil
}

// GetMobile 用 App 端一键登录 SDK 返回的 AccessToken 换取本机手机号。
// AccessToken 一次有效：电信 10 分钟、联通 30 分钟、移动 2 分钟。
func (c *Client) GetMobile(ctx context.Context, accessToken string) (string, error) {
	if !c.configured() {
		return "", ErrNotConfigured
	}
	resp, err := c.client.GetMobile(&dypnsapi.GetMobileRequest{
		AccessToken: tea.String(strings.TrimSpace(accessToken)),
	})
	if err != nil {
		return "", fmt.Errorf("aliyun get mobile: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return "", errors.New("aliyun get mobile: empty response")
	}
	if code := tea.StringValue(resp.Body.Code); code != "" && code != "OK" {
		return "", errors.New("aliyun get mobile: " + firstNonEmpty(tea.StringValue(resp.Body.Message), code))
	}
	if resp.Body.GetMobileResultDTO == nil {
		return "", errors.New("aliyun get mobile: empty mobile")
	}
	return tea.StringValue(resp.Body.GetMobileResultDTO.Mobile), nil
}

// MobileVerifyResult 是本机号码校验结果。
type MobileVerifyResult struct {
	// Result 取值 PASS（一致）、REJECT（不一致）、UNKNOWN（无法判断）。
	Result   string
	VerifyID string
}

// Matched 表示本机号码与目标手机号一致。
func (r MobileVerifyResult) Matched() bool { return r.Result == "PASS" }

// VerifyMobile 校验 SDK 取号得到的 AccessCode 与给定手机号是否一致。
// AccessCode 一次有效：电信 10 分钟、联通 30 分钟、移动 2 分钟。
func (c *Client) VerifyMobile(ctx context.Context, accessCode, phone string) (MobileVerifyResult, error) {
	if !c.configured() {
		return MobileVerifyResult{}, ErrNotConfigured
	}
	resp, err := c.client.VerifyMobile(&dypnsapi.VerifyMobileRequest{
		AccessCode:  tea.String(strings.TrimSpace(accessCode)),
		PhoneNumber: tea.String(strings.TrimSpace(phone)),
	})
	if err != nil {
		return MobileVerifyResult{}, fmt.Errorf("aliyun verify mobile: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return MobileVerifyResult{}, errors.New("aliyun verify mobile: empty response")
	}
	if code := tea.StringValue(resp.Body.Code); code != "" && code != "OK" {
		return MobileVerifyResult{}, errors.New("aliyun verify mobile: " + firstNonEmpty(tea.StringValue(resp.Body.Message), code))
	}
	result := MobileVerifyResult{}
	if resp.Body.GateVerifyResultDTO != nil {
		result.Result = tea.StringValue(resp.Body.GateVerifyResultDTO.VerifyResult)
		result.VerifyID = tea.StringValue(resp.Body.GateVerifyResultDTO.VerifyId)
	}
	return result, nil
}

// SdkTokens 是 Android 短信验证码 SDK 初始化所需的 Token 与 STS 临时凭证。
type SdkTokens struct {
	BizToken           string `json:"biz_token"`
	ExpireTime         int64  `json:"expire_time"` // 毫秒时间戳
	StsAccessKeyID     string `json:"sts_access_key_id"`
	StsAccessKeySecret string `json:"sts_access_key_secret"`
	StsToken           string `json:"sts_token"`
}

const (
	sdkTokenExpireSeconds = int64(3600)
	sdkCodeExpireSeconds  = int32(60)
)

// GetSmsAuthTokens 获取短信验证码 SDK 所需的 BizToken 与 STS 临时凭证。
// packageName 为 App 包名，appSignature 为 App 签名（SDK 内 PackageUtils.getSign 的结果）。
func (c *Client) GetSmsAuthTokens(ctx context.Context, packageName, appSignature, sceneCode string) (*SdkTokens, error) {
	if !c.SDKEnabled() {
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(sceneCode) == "" {
		sceneCode = c.scene
	}
	if strings.TrimSpace(packageName) == "" || strings.TrimSpace(appSignature) == "" {
		return nil, errors.New("package_name and sign_name are required")
	}
	resp, err := c.client.GetSmsAuthTokens(&dypnsapi.GetSmsAuthTokensRequest{
		SceneCode:       tea.String(sceneCode),
		OsType:          tea.String("Android"),
		PackageName:     tea.String(packageName),
		SignName:        tea.String(appSignature),
		Expire:          tea.Int64(sdkTokenExpireSeconds),
		SmsCodeExpire:   tea.Int32(sdkCodeExpireSeconds),
		SmsTemplateCode: tea.String(c.template),
	})
	if err != nil {
		return nil, fmt.Errorf("aliyun get sms auth tokens: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("aliyun get sms auth tokens: empty response")
	}
	if code := tea.StringValue(resp.Body.Code); code != "" && code != "OK" {
		return nil, errors.New("aliyun get sms auth tokens: " + firstNonEmpty(tea.StringValue(resp.Body.Message), code))
	}
	data := resp.Body.Data
	if data == nil {
		return nil, errors.New("aliyun get sms auth tokens: empty data")
	}
	return &SdkTokens{
		BizToken:           tea.StringValue(data.BizToken),
		ExpireTime:         tea.Int64Value(data.ExpireTime),
		StsAccessKeyID:     tea.StringValue(data.StsAccessKeyId),
		StsAccessKeySecret: tea.StringValue(data.StsAccessKeySecret),
		StsToken:           tea.StringValue(data.StsToken),
	}, nil
}

// VerifySmsCode 校验 SDK 流程下用户填写的短信验证码，返回是否通过。
func (c *Client) VerifySmsCode(ctx context.Context, phone, code, smsToken string) (bool, error) {
	if !c.configured() {
		return false, ErrNotConfigured
	}
	if strings.TrimSpace(phone) == "" || strings.TrimSpace(code) == "" || strings.TrimSpace(smsToken) == "" {
		return false, errors.New("phone, sms_code and sms_token are required")
	}
	resp, err := c.client.VerifySmsCode(&dypnsapi.VerifySmsCodeRequest{
		PhoneNumber: tea.String(strings.TrimSpace(phone)),
		SmsCode:     tea.String(strings.TrimSpace(code)),
		SmsToken:    tea.String(strings.TrimSpace(smsToken)),
	})
	if err != nil {
		return false, fmt.Errorf("aliyun verify sms code: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return false, errors.New("aliyun verify sms code: empty response")
	}
	if code := tea.StringValue(resp.Body.Code); code != "" && code != "OK" {
		return false, errors.New("aliyun verify sms code: " + firstNonEmpty(tea.StringValue(resp.Body.Message), code))
	}
	return tea.BoolValue(resp.Body.Data), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
