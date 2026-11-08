package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackmengtag/SonicHub_api/internal/store"
)

func queryString(r *http.Request, key string) string {
	return strings.TrimSpace(r.URL.Query().Get(key))
}

func failStore(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case errors.Is(err, store.ErrSelfReview), errors.Is(err, store.ErrInsufficientFunds),
		errors.Is(err, store.ErrInvalidState), errors.Is(err, store.ErrDuplicate):
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
	default:
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
}

/* ---------------- 用户管理 ---------------- */

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListAdminUsers(r.Context(), queryString(r, "query"), queryString(r, "status"), queryInt(r, "limit", 100))
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) adminUserDetail(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	id := pathID(r, "id")
	u, err := s.store.GetAdminUser(r.Context(), id)
	if err != nil {
		failStore(w, err)
		return
	}
	works, _ := s.store.ListAdminWorks(r.Context(), "", "", "", 200)
	own := []store.AdminWorkItem{}
	for _, x := range works {
		if x.OwnerID == id {
			own = append(own, x)
			if len(own) >= 10 {
				break
			}
		}
	}
	dispositions, _ := s.store.ListUserDispositions(r.Context(), id, 20)
	ok(w, map[string]any{"user": u, "works": own, "dispositions": dispositions})
}

func (s *Server) adminUserDecision(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	switch req.Action {
	case "warn", "limit", "ban", "unban":
	default:
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "action must be warn, limit, ban or unban")
		return
	}
	userID := pathID(r, "id")
	if userID == id.UserID {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "不能处置当前登录账号")
		return
	}
	next, err := s.store.DecideUser(r.Context(), userID, id.UserID, req.Action, req.Reason)
	if err != nil {
		failStore(w, err)
		return
	}
	title, body := "账号处置通知", req.Reason
	switch req.Action {
	case "warn":
		title = "账号警告"
	case "limit":
		title = "账号发布权限受限"
	case "ban":
		title = "账号已被封禁"
	case "unban":
		title = "账号已解封"
	}
	if strings.TrimSpace(body) == "" {
		body = "如有疑问请通过客服渠道申诉"
	}
	_ = s.store.Notify(r.Context(), userID, "system", "", title, body, "user", userID, "disposition:"+userID)
	ok(w, map[string]any{"ok": true, "status": next})
}

/* ---------------- 作品管理 ---------------- */

func (s *Server) adminWorks(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListAdminWorks(r.Context(), queryString(r, "query"), queryString(r, "status"), queryString(r, "visibility"), queryInt(r, "limit", 200))
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) adminWorkDetail(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	d, err := s.store.GetAdminWorkDetail(r.Context(), pathID(r, "id"))
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, d)
}

func (s *Server) adminWorkDecision(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	switch req.Action {
	case "unpublish", "restore", "pin", "unpin", "delete":
	default:
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "action must be unpublish, restore, pin, unpin or delete")
		return
	}
	workID := pathID(r, "id")
	owner, title, err := s.store.DecideWorkAction(r.Context(), workID, id.UserID, req.Action, req.Reason)
	if err != nil {
		failStore(w, err)
		return
	}
	if owner != "" && req.Action != "pin" && req.Action != "unpin" {
		notice, body := "作品已下架", req.Reason
		switch req.Action {
		case "restore":
			notice = "作品已恢复发布"
		case "delete":
			notice = "作品已删除"
		}
		if strings.TrimSpace(body) == "" {
			body = "《" + title + "》" + notice
		}
		_ = s.store.Notify(r.Context(), owner, "work", "", notice, body, "work", workID, "work-ops:"+workID)
	}
	ok(w, map[string]any{"ok": true})
}

/* ---------------- 积分运营 ---------------- */

func (s *Server) adminCreditSummary(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	x, err := s.store.AdminCreditSummary(r.Context())
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, x)
}

func (s *Server) adminCreditLedger(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListAdminLedger(r.Context(), queryString(r, "query"), queryInt(r, "limit", 100))
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) adminCreditAdjustments(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListCreditAdjustments(r.Context(), queryString(r, "status"), queryInt(r, "limit", 100))
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) adminCreateCreditAdjustment(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		UserID string `json:"user_id"`
		Amount int64  `json:"amount"`
		Kind   string `json:"kind"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "user_id is required")
		return
	}
	if req.Amount == 0 {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "amount must not be zero")
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "reason is required")
		return
	}
	if req.Kind == "" {
		req.Kind = "客诉补偿"
	}
	a, err := s.store.CreateCreditAdjustment(r.Context(), id.UserID, req.UserID, req.Amount, req.Kind, req.Reason)
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, a)
}

func (s *Server) adminDecideCreditAdjustment(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Decision != "approve" && req.Decision != "reject" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "decision must be approve or reject")
		return
	}
	adjustmentID := pathID(r, "id")
	a, err := s.store.GetCreditAdjustment(r.Context(), adjustmentID)
	if err != nil {
		failStore(w, err)
		return
	}
	if err := s.store.DecideCreditAdjustment(r.Context(), adjustmentID, id.UserID, req.Decision, req.Reason); err != nil {
		failStore(w, err)
		return
	}
	if req.Decision == "approve" {
		body := "积分调整已生效"
		if req.Reason != "" {
			body = req.Reason
		}
		_ = s.store.Notify(r.Context(), a.UserID, "credit", "", "积分调整到账", body, "adjustment", adjustmentID, "adjust:"+adjustmentID)
	}
	ok(w, map[string]any{"ok": true})
}

/* ---------------- 消息运营 ---------------- */

func (s *Server) adminMessages(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListAnnouncements(r.Context(), queryString(r, "status"), queryInt(r, "limit", 100))
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) adminCreateMessage(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Title    string   `json:"title"`
		Body     string   `json:"body"`
		Audience string   `json:"audience"`
		Channels []string `json:"channels"`
		Link     string   `json:"link"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "title and body are required")
		return
	}
	if req.Audience == "" {
		req.Audience = "all"
	}
	if len(req.Channels) == 0 {
		req.Channels = []string{"站内"}
	}
	a, err := s.store.CreateAnnouncement(r.Context(), id.UserID, req.Title, req.Body, req.Audience, req.Link, req.Channels)
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, a)
}

func (s *Server) adminSendMessage(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	n, err := s.store.SendAnnouncement(r.Context(), pathID(r, "id"), id.UserID)
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"ok": true, "recipients": n})
}

func (s *Server) adminTemplates(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListMessageTemplates(r.Context())
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) adminCreateTemplate(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Name    string `json:"name"`
		Scene   string `json:"scene"`
		Channel string `json:"channel"`
		Content string `json:"content"`
		Enabled *bool  `json:"enabled"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Content) == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "name and content are required")
		return
	}
	if req.Channel == "" {
		req.Channel = "站内"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	t, err := s.store.CreateMessageTemplate(r.Context(), id.UserID, req.Name, req.Scene, req.Channel, req.Content, enabled)
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, t)
}

func (s *Server) adminUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Name    string `json:"name"`
		Scene   string `json:"scene"`
		Channel string `json:"channel"`
		Content string `json:"content"`
		Enabled *bool  `json:"enabled"`
	}
	if !decode(w, r, &req) {
		return
	}
	existing, err := s.store.GetMessageTemplate(r.Context(), pathID(r, "id"))
	if err != nil {
		failStore(w, err)
		return
	}
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Content == "" {
		req.Content = existing.Content
	}
	if req.Scene == "" {
		req.Scene = existing.Scene
	}
	if req.Channel == "" {
		req.Channel = existing.Channel
	}
	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	t, err := s.store.UpdateMessageTemplate(r.Context(), id.UserID, existing.ID, req.Name, req.Scene, req.Channel, req.Content, enabled)
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, t)
}

/* ---------------- 广场运营 ---------------- */

func (s *Server) adminPlacements(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListPlazaPlacements(r.Context())
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) adminCreatePlacement(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Name     string `json:"name"`
		Position string `json:"position"`
		WorkID   string `json:"work_id"`
		Schedule string `json:"schedule"`
		Status   string `json:"status"`
		Owner    string `json:"owner"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "name is required")
		return
	}
	if req.Status == "" {
		req.Status = "scheduled"
	}
	p, err := s.store.CreatePlazaPlacement(r.Context(), id.UserID, req.Name, req.Position, req.WorkID, req.Schedule, req.Status, req.Owner)
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, p)
}

func (s *Server) adminUpdatePlacement(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Name     string `json:"name"`
		Position string `json:"position"`
		WorkID   string `json:"work_id"`
		Schedule string `json:"schedule"`
		Status   string `json:"status"`
		Owner    string `json:"owner"`
	}
	if !decode(w, r, &req) {
		return
	}
	existing, err := s.store.GetPlazaPlacement(r.Context(), pathID(r, "id"))
	if err != nil {
		failStore(w, err)
		return
	}
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Position == "" {
		req.Position = existing.Position
	}
	if req.WorkID == "" {
		req.WorkID = existing.WorkID
	}
	if req.Schedule == "" {
		req.Schedule = existing.Schedule
	}
	if req.Status == "" {
		req.Status = existing.Status
	}
	if req.Owner == "" {
		req.Owner = existing.Owner
	}
	p, err := s.store.UpdatePlazaPlacement(r.Context(), id.UserID, existing.ID, req.Name, req.Position, req.WorkID, req.Schedule, req.Status, req.Owner)
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, p)
}

func (s *Server) adminTopics(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListPlazaTopics(r.Context())
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) adminCreateTopic(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Schedule    string `json:"schedule"`
		Status      string `json:"status"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "name is required")
		return
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	t, err := s.store.CreatePlazaTopic(r.Context(), id.UserID, req.Name, req.Description, req.Schedule, req.Status)
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, t)
}

func (s *Server) adminUpdateTopic(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Schedule    string `json:"schedule"`
		Status      string `json:"status"`
	}
	if !decode(w, r, &req) {
		return
	}
	existing, err := s.store.GetPlazaTopic(r.Context(), pathID(r, "id"))
	if err != nil {
		failStore(w, err)
		return
	}
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Description == "" {
		req.Description = existing.Description
	}
	if req.Schedule == "" {
		req.Schedule = existing.Schedule
	}
	if req.Status == "" {
		req.Status = existing.Status
	}
	t, err := s.store.UpdatePlazaTopic(r.Context(), id.UserID, existing.ID, req.Name, req.Description, req.Schedule, req.Status)
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, t)
}

func (s *Server) adminRanking(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListPlazaRanking(r.Context(), queryInt(r, "limit", 10))
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"items": items})
}

/* ---------------- 审计日志 ---------------- */

func (s *Server) adminAuditLogs(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListAuditLogs(r.Context(), queryString(r, "query"), queryString(r, "type"), queryInt(r, "limit", 150))
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) adminAuditLogDetail(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	x, err := s.store.GetAuditLog(r.Context(), pathID(r, "id"))
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, x)
}
