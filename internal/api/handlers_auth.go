package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/model"
	"github.com/jackmengtag/SonicHub_api/internal/platform/auth"
	"github.com/jackmengtag/SonicHub_api/internal/platform/rbac"
	"github.com/jackmengtag/SonicHub_api/internal/store"
)

type loginRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decode(w, r, &req) {
		return
	}
	req.Phone = strings.TrimSpace(req.Phone)
	if !validPhone(req.Phone) {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "请输入 11 位手机号")
		return
	}
	verified, err := s.verifyLoginCode(r.Context(), req.Phone, req.Code)
	if err != nil {
		fail(w, http.StatusBadGateway, "SMS_VERIFY_FAILED", err.Error())
		return
	}
	if !verified {
		message := "验证码错误或已过期"
		if s.cfg.AllowDemoCode || s.pns == nil || !s.pns.Enabled() {
			message = "验证码错误，演示验证码为 " + s.cfg.DemoCode
		}
		fail(w, http.StatusUnauthorized, "UNAUTHENTICATED", message)
		return
	}

	result, err := s.authenticatePhone(r, req.Phone)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, result)
}

// authenticatePhone 按手机号完成登录或自动注册，并签发 Token。
func (s *Server) authenticatePhone(r *http.Request, phone string) (map[string]any, error) {
	u, err := s.store.FindUserByPhone(r.Context(), phone)
	if errors.Is(err, store.ErrNotFound) {
		created, cerr := s.store.CreateUser(r.Context(), phone, "")
		if cerr != nil {
			return nil, cerr
		}
		u = created.User
	} else if err != nil {
		return nil, err
	}
	tenantID, err := s.store.TenantOf(r.Context(), u.ID)
	if err != nil {
		return nil, err
	}
	return s.issueTokens(r, u, tenantID)
}

func (s *Server) issueTokens(r *http.Request, u *model.User, tenantID string) (map[string]any, error) {
	access, err := auth.NewAccessToken(s.cfg.JWTSecret, u.ID, tenantID, u.Role, s.cfg.AccessTTL)
	if err != nil {
		return nil, err
	}
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	sessionID := store.NewID("s_")
	expires := time.Now().Add(s.cfg.RefreshTTL)
	if err := s.store.CreateSession(r.Context(), sessionID, u.ID, tenantID, hash, expires); err != nil {
		return nil, err
	}
	return map[string]any{
		"access_token":  access,
		"refresh_token": raw,
		"expires_in":    int(s.cfg.AccessTTL.Seconds()),
		"user":          u,
		"tenant_id":     tenantID,
		"role_name":     rbac.RoleName(u.Role),
		"permissions":   rbac.PermissionsFor(u.Role),
	}, nil
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decode(w, r, &req) {
		return
	}
	hash := auth.HashToken(req.RefreshToken)
	session, err := s.store.FindSession(r.Context(), hash)
	if errors.Is(err, store.ErrNotFound) || (err == nil && (session.Revoked.Valid || time.Now().After(session.ExpiresAt))) {
		fail(w, http.StatusUnauthorized, "UNAUTHENTICATED", "refresh token is invalid")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	u, err := s.store.GetUser(r.Context(), session.UserID)
	if err != nil {
		fail(w, http.StatusUnauthorized, "UNAUTHENTICATED", "user not found")
		return
	}
	_ = s.store.RevokeSession(r.Context(), session.ID)
	result, err := s.issueTokens(r, u, session.TenantID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, result)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.RefreshToken != "" {
		_ = s.store.RevokeSessionByHash(r.Context(), auth.HashToken(req.RefreshToken))
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	u, err := s.store.GetUser(r.Context(), id.UserID)
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}
	musician, _ := s.store.GetMusician(r.Context(), id.UserID)
	credits, _ := s.store.Account(r.Context(), id.TenantID)
	works, following, followers, friends, _ := s.store.Counts(r.Context(), id.UserID)
	unread, _ := s.store.UnreadCount(r.Context(), id.UserID)
	ok(w, map[string]any{
		"user":     u,
		"musician": musician,
		"credits":  credits,
		"counts": map[string]int{
			"works": works, "following": following, "followers": followers, "friends": friends,
		},
		"unread_notifications": unread,
		"role_name":            rbac.RoleName(u.Role),
		"permissions":          rbac.PermissionsFor(u.Role),
	})
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		DisplayName string `json:"display_name"`
		Bio         string `json:"bio"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.DisplayName) == "" || len([]rune(req.DisplayName)) > 20 {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "昵称长度需为 1–20 字")
		return
	}
	if err := s.store.UpdateProfile(r.Context(), id.UserID, req.DisplayName, req.Bio); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	u, _ := s.store.GetUser(r.Context(), id.UserID)
	ok(w, u)
}

func (s *Server) saveInterests(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		Genres []string `json:"genres"`
	}
	if !decode(w, r, &req) {
		return
	}
	if len(req.Genres) < 3 {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "请至少选择 3 个风格")
		return
	}
	if err := s.store.SaveInterests(r.Context(), id.UserID, req.Genres); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"genres": req.Genres})
}

func (s *Server) userProfile(w http.ResponseWriter, r *http.Request) {
	viewer, _ := identity(r)
	target := pathID(r, "id")
	card, err := s.store.GetUserCard(r.Context(), viewer.UserID, target)
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}
	musician, _ := s.store.GetMusician(r.Context(), target)
	works, following, followers, friends, _ := s.store.Counts(r.Context(), target)
	ok(w, map[string]any{
		"user": card, "musician": musician,
		"counts": map[string]int{"works": works, "following": following, "followers": followers, "friends": friends},
	})
}

func (s *Server) musicianProfile(w http.ResponseWriter, r *http.Request) {
	viewer, _ := identity(r)
	target := pathID(r, "id")
	card, err := s.store.GetUserCard(r.Context(), viewer.UserID, target)
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}
	musician, _ := s.store.GetMusician(r.Context(), target)
	works, _ := s.store.ListWorks(r.Context(), store.WorkFilter{
		ViewerID: viewer.UserID, OwnerID: target, Status: "published", Limit: 30,
	})
	ok(w, map[string]any{"user": card, "musician": musician, "works": works})
}

func (s *Server) applyMusician(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		StageName string   `json:"stage_name"`
		Bio       string   `json:"bio"`
		Genres    []string `json:"genres"`
	}
	if !decode(w, r, &req) {
		return
	}
	if len([]rune(req.StageName)) < 2 || len([]rune(req.StageName)) > 20 {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "艺名长度需为 2–20 字")
		return
	}
	if len(req.Genres) == 0 || len(req.Genres) > 3 {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "请选择 1–3 个擅长风格")
		return
	}
	if err := s.store.ApplyMusician(r.Context(), id.UserID, req.StageName, req.Bio, req.Genres); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	_ = s.store.Notify(r.Context(), id.UserID, "system", "", "音乐人认证已提交", "预计 1–3 个工作日完成审核", "musician", id.UserID, "musician:"+id.UserID)
	ok(w, map[string]any{"status": "pending"})
}

func (s *Server) myApplication(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	m, err := s.store.GetMusician(r.Context(), id.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, m)
}
