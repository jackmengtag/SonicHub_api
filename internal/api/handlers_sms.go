package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/platform/pns"
	"github.com/jackmengtag/SonicHub_api/internal/store"
)

type smsCodeRequest struct {
	Phone string `json:"phone"`
}

func validPhone(phone string) bool {
	if len(phone) != 11 || !strings.HasPrefix(phone, "1") {
		return false
	}
	for _, r := range phone {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// sendSMSCode 下发短信验证码。
// SMS_PROVIDER=dysms 时使用短信服务 + 自建验证码；默认 dypns 走号码认证服务。
func (s *Server) sendSMSCode(w http.ResponseWriter, r *http.Request) {
	var req smsCodeRequest
	if !decode(w, r, &req) {
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if !validPhone(phone) {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "请输入 11 位手机号")
		return
	}
	if s.cfg.SMSProvider == "dysms" && s.dysms != nil && s.dysms.Enabled() {
		code, err := randomSMSCode()
		if err != nil {
			fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", "生成验证码失败")
			return
		}
		if err := s.store.SaveSMSCode(r.Context(), phone, code, 5*time.Minute); err != nil {
			if errors.Is(err, store.ErrTooFrequent) {
				fail(w, http.StatusTooManyRequests, "TOO_MANY_REQUESTS", "验证码发送过于频繁，请 60 秒后再试")
				return
			}
			fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		bizID, err := s.dysms.Send(r.Context(), phone, code)
		if err != nil {
			fail(w, http.StatusBadGateway, "SMS_SEND_FAILED", err.Error())
			return
		}
		ok(w, map[string]any{
			"sent": true, "provider": "dysms", "biz_id": bizID,
			"interval_seconds": 60, "valid_seconds": 300,
		})
		return
	}
	if s.pns == nil || !s.pns.Enabled() {
		ok(w, map[string]any{
			"sent": false, "demo": true, "demo_code": s.cfg.DemoCode,
			"message": "未配置阿里云短信密钥，当前为演示模式，验证码固定为 " + s.cfg.DemoCode,
		})
		return
	}
	bizID, err := s.pns.Send(r.Context(), phone)
	if err != nil {
		fail(w, http.StatusBadGateway, "SMS_SEND_FAILED", err.Error())
		return
	}
	ok(w, map[string]any{
		"sent": true, "provider": "dypns", "biz_id": bizID,
		"interval_seconds": 60, "valid_seconds": 300,
	})
}

// verifyLoginCode 校验登录验证码：dysms 走本地校验，dypns 走阿里云核验，未配置时用演示验证码。
// ALLOW_DEMO_CODE=true 时，本地调试可继续使用演示验证码直接登录。
func (s *Server) verifyLoginCode(ctx context.Context, phone, code string) (bool, error) {
	if s.cfg.AllowDemoCode && strings.TrimSpace(code) == s.cfg.DemoCode {
		return true, nil
	}
	if s.cfg.SMSProvider == "dysms" && s.dysms != nil && s.dysms.Enabled() {
		return s.store.VerifySMSCode(ctx, phone, strings.TrimSpace(code))
	}
	if s.pns == nil || !s.pns.Enabled() {
		return strings.TrimSpace(code) == s.cfg.DemoCode, nil
	}
	if strings.TrimSpace(code) == "" {
		return false, nil
	}
	return s.pns.Verify(ctx, phone, code)
}

func randomSMSCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// smsSDKTokens 对应 SDK 的 TokenUpdater：下发 BizToken 与 STS 临时凭证。
// App 端需要传 package_name（包名）和 sign_name（APK 签名，SDK 的 PackageUtils.getSign 结果）。
func (s *Server) smsSDKTokens(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SceneCode   string `json:"scene_code"`
		PackageName string `json:"package_name"`
		SignName    string `json:"sign_name"`
	}
	if !decode(w, r, &req) {
		return
	}
	if s.pns == nil || !s.pns.SDKEnabled() {
		fail(w, http.StatusNotImplemented, "SMS_SDK_UNAVAILABLE",
			"未配置阿里云短信认证方案，请设置 ALIYUN_SMS_SCENE_CODE 与 ALIYUN_SMS_TEMPLATE_CODE 后重启")
		return
	}
	tokens, err := s.pns.GetSmsAuthTokens(r.Context(),
		strings.TrimSpace(req.PackageName), strings.TrimSpace(req.SignName), strings.TrimSpace(req.SceneCode))
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, pns.ErrNotConfigured) {
			status = http.StatusNotImplemented
		}
		fail(w, status, "SMS_TOKEN_FAILED", err.Error())
		return
	}
	ok(w, map[string]any{
		"biz_token":             tokens.BizToken,
		"expire_time":           tokens.ExpireTime,
		"sts_access_key_id":     tokens.StsAccessKeyID,
		"sts_access_key_secret": tokens.StsAccessKeySecret,
		"sts_token":             tokens.StsToken,
		"scene_code":            s.cfg.AliyunSMSSceneCode,
		"sms_code_expire":       60,
	})
}

// smsSDKVerify 校验 SDK 流程提交的验证码，通过后登录/自动注册并签发 Token。
func (s *Server) smsSDKVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Phone    string `json:"phone"`
		SmsCode  string `json:"sms_code"`
		SmsToken string `json:"sms_token"`
	}
	if !decode(w, r, &req) {
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if !validPhone(phone) {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "请输入 11 位手机号")
		return
	}
	if strings.TrimSpace(req.SmsCode) == "" || strings.TrimSpace(req.SmsToken) == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "sms_code and sms_token are required")
		return
	}
	if s.pns == nil || !s.pns.Available() {
		fail(w, http.StatusNotImplemented, "SMS_SDK_UNAVAILABLE",
			"未配置阿里云短信认证服务，请设置 ALIYUN_ACCESS_KEY_ID/SECRET 后重启")
		return
	}
	matched, err := s.pns.VerifySmsCode(r.Context(), phone, req.SmsCode, req.SmsToken)
	if err != nil {
		fail(w, http.StatusBadGateway, "SMS_VERIFY_FAILED", err.Error())
		return
	}
	if !matched {
		fail(w, http.StatusUnauthorized, "UNAUTHENTICATED", "验证码错误或已过期")
		return
	}
	result, err := s.authenticatePhone(r, phone)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, result)
}
