package api

import (
	"net/http"
	"strings"
)

// verifyMobile 校验「本机号码」与用户提交的手机号是否一致（本机号码校验认证）。
func (s *Server) verifyMobile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccessToken string `json:"access_token"`
		Phone       string `json:"phone"`
	}
	if !decode(w, r, &req) {
		return
	}
	token := strings.TrimSpace(req.AccessToken)
	phone := strings.TrimSpace(req.Phone)
	if token == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "access_token is required")
		return
	}
	if !validPhone(phone) {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "请输入 11 位手机号")
		return
	}
	if s.pns == nil || !s.pns.Available() {
		fail(w, http.StatusNotImplemented, "ONE_CLICK_UNAVAILABLE",
			"未配置阿里云号码认证服务，请设置 ALIYUN_ACCESS_KEY_ID/SECRET 后重启")
		return
	}
	result, err := s.pns.VerifyMobile(r.Context(), token, phone)
	if err != nil {
		fail(w, http.StatusBadGateway, "MOBILE_VERIFY_FAILED", err.Error())
		return
	}
	ok(w, map[string]any{
		"matched":   result.Matched(),
		"result":    result.Result,
		"verify_id": result.VerifyID,
	})
}

// oneClickLogin 用 App 端一键登录 SDK 返回的 AccessToken 换取本机号码，并完成登录/自动注册。
// 客户端流程：拉起 SDK 授权页 → 用户同意 → SDK 返回 AccessToken → 调用本接口。
func (s *Server) oneClickLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccessToken string `json:"access_token"`
	}
	if !decode(w, r, &req) {
		return
	}
	token := strings.TrimSpace(req.AccessToken)
	if token == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "access_token is required")
		return
	}
	if s.pns == nil || !s.pns.Available() {
		fail(w, http.StatusNotImplemented, "ONE_CLICK_UNAVAILABLE",
			"未配置阿里云号码认证服务，请设置 ALIYUN_ACCESS_KEY_ID/SECRET 后重启")
		return
	}
	mobile, err := s.pns.GetMobile(r.Context(), token)
	if err != nil {
		fail(w, http.StatusBadGateway, "ONE_CLICK_FAILED", err.Error())
		return
	}
	if !validPhone(mobile) {
		fail(w, http.StatusBadGateway, "ONE_CLICK_FAILED", "未能获取有效手机号")
		return
	}
	result, err := s.authenticatePhone(r, mobile)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, result)
}
