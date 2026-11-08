package api

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) follow(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	target := pathID(r, "id")
	if target == id.UserID {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "不能关注自己")
		return
	}
	if _, err := s.store.GetUser(r.Context(), target); err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}
	if s.store.IsBlocked(r.Context(), id.UserID, target) {
		fail(w, http.StatusForbidden, "FORBIDDEN", "无法关注该用户")
		return
	}
	if err := s.store.Follow(r.Context(), id.UserID, target); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	_ = s.store.Notify(r.Context(), target, "interaction", id.UserID, "有人关注了你", "", "user", id.UserID, "follow:"+id.UserID)
	friend := s.store.IsFriend(r.Context(), id.UserID, target)
	if friend {
		_ = s.store.Notify(r.Context(), target, "interaction", id.UserID, "你们成为朋友", "互相关注后可以查看朋友可见作品并私信", "user", id.UserID, "friend:"+id.UserID+":"+target)
	}
	ok(w, map[string]any{"following": true, "friend": friend})
}

func (s *Server) unfollow(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	target := pathID(r, "id")
	if err := s.store.Unfollow(r.Context(), id.UserID, target); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"following": false, "friend": s.store.IsFriend(r.Context(), id.UserID, target)})
}

func (s *Server) meFollowing(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	items, err := s.store.Following(r.Context(), id.UserID, id.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) meFollowers(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	items, err := s.store.Followers(r.Context(), id.UserID, id.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) meFriends(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	items, err := s.store.Friends(r.Context(), id.UserID, id.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) userFollowing(w http.ResponseWriter, r *http.Request) {
	viewer, _ := identity(r)
	target := pathID(r, "id")
	items, err := s.store.Following(r.Context(), viewer.UserID, target)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) userFollowers(w http.ResponseWriter, r *http.Request) {
	viewer, _ := identity(r)
	target := pathID(r, "id")
	items, err := s.store.Followers(r.Context(), viewer.UserID, target)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) blockUser(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	if err := s.store.Block(r.Context(), id.UserID, pathID(r, "id")); err != nil {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	ok(w, map[string]any{"blocked": true})
}

func (s *Server) unblockUser(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	if err := s.store.Unblock(r.Context(), id.UserID, pathID(r, "id")); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"blocked": false})
}

/* ---------------- notifications ---------------- */

func (s *Server) notifications(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	items, err := s.store.ListNotifications(r.Context(), id.UserID, r.URL.Query().Get("filter"), queryInt(r, "limit", 30))
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	unread, _ := s.store.UnreadCount(r.Context(), id.UserID)
	ok(w, map[string]any{"items": items, "unread": unread})
}

func (s *Server) readNotifications(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	_ = decode(w, r, &req)
	if req.ID != "" {
		_ = s.store.MarkNotificationRead(r.Context(), req.ID, id.UserID)
	} else {
		_ = s.store.MarkAllRead(r.Context(), id.UserID)
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) readNotification(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	_ = s.store.MarkNotificationRead(r.Context(), pathID(r, "id"), id.UserID)
	ok(w, map[string]any{"ok": true})
}

/* ---------------- conversations ---------------- */

func (s *Server) conversations(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	items, err := s.store.Conversations(r.Context(), id.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		UserID string `json:"user_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.UserID == "" || req.UserID == id.UserID {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid user")
		return
	}
	if !s.store.IsFriend(r.Context(), id.UserID, req.UserID) {
		fail(w, http.StatusForbidden, "FORBIDDEN", "仅朋友之间可以发起私信")
		return
	}
	convID, err := s.store.EnsureDirectConversation(r.Context(), id.UserID, req.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"id": convID})
}

func (s *Server) messages(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	convID := pathID(r, "id")
	if !s.store.IsConversationMember(r.Context(), convID, id.UserID) {
		fail(w, http.StatusForbidden, "FORBIDDEN", "not a conversation member")
		return
	}
	items, err := s.store.Messages(r.Context(), convID, queryInt(r, "limit", 50))
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	_ = s.store.MarkConversationRead(r.Context(), convID, id.UserID)
	ok(w, map[string]any{"items": items})
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	convID := pathID(r, "id")
	if !s.store.IsConversationMember(r.Context(), convID, id.UserID) {
		fail(w, http.StatusForbidden, "FORBIDDEN", "not a conversation member")
		return
	}
	peer, err := s.store.ConversationPeer(r.Context(), convID, id.UserID)
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "conversation not found")
		return
	}
	if !s.store.IsFriend(r.Context(), id.UserID, peer) {
		fail(w, http.StatusForbidden, "FORBIDDEN", "仅朋友之间可以私信")
		return
	}
	var req struct {
		Type   string `json:"type"`
		Text   string `json:"text"`
		WorkID string `json:"work_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Type == "" {
		req.Type = "text"
	}
	if req.Type == "text" && strings.TrimSpace(req.Text) == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "消息不能为空")
		return
	}
	if len([]rune(req.Text)) > 1000 {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "消息过长")
		return
	}
	if req.Type == "song" && req.WorkID == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "work_id is required")
		return
	}
	if req.Type == "song" {
		if _, err := s.store.GetWork(r.Context(), req.WorkID, id.UserID); err != nil {
			fail(w, http.StatusNotFound, "NOT_FOUND", "work not found")
			return
		}
	}
	m, err := s.store.SendMessage(r.Context(), convID, id.UserID, req.Type, req.Text, req.WorkID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, m)
}

func (s *Server) readConversation(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	_ = s.store.MarkConversationRead(r.Context(), pathID(r, "id"), id.UserID)
	ok(w, map[string]any{"ok": true})
}

/* ---------------- reports / appeals ---------------- */

func (s *Server) createReport(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		Reason     string `json:"reason"`
		Detail     string `json:"detail"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.TargetType == "" || req.TargetID == "" || req.Reason == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "target and reason are required")
		return
	}
	if err := s.store.CreateReport(r.Context(), id.UserID, req.TargetType, req.TargetID, req.Reason, req.Detail); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"submitted": true})
}

func (s *Server) createAppeal(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		WorkID string `json:"work_id"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.WorkID == "" || strings.TrimSpace(req.Reason) == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "work_id and reason are required")
		return
	}
	work, err := s.store.GetWorkAny(r.Context(), req.WorkID)
	if err != nil || work.OwnerID != id.UserID {
		fail(w, http.StatusNotFound, "NOT_FOUND", "work not found")
		return
	}
	appealID, err := s.store.CreateAppeal(r.Context(), req.WorkID, id.UserID, req.Reason)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	_ = s.store.Notify(r.Context(), id.UserID, "work", "", "申诉已提交", "我们将在 1–3 个工作日内反馈", "appeal", appealID, "appeal:"+appealID)
	ok(w, map[string]any{"id": appealID, "status": "open"})
}

func (s *Server) myAppeals(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	items, err := s.store.MyAppeals(r.Context(), id.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
