package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/core"
	"github.com/jackmengtag/SonicHub_api/internal/store"
)

func (s *Server) listWorks(w http.ResponseWriter, r *http.Request) {
	viewer, _ := identity(r)
	owner := r.URL.Query().Get("owner")
	if owner == "me" {
		if viewer.UserID == "" {
			fail(w, http.StatusUnauthorized, "UNAUTHENTICATED", "authorization required")
			return
		}
		owner = viewer.UserID
	}
	f := store.WorkFilter{
		ViewerID: viewer.UserID,
		OwnerID:  owner,
		Status:   r.URL.Query().Get("status"),
		Genre:    r.URL.Query().Get("genre"),
		Sort:     r.URL.Query().Get("sort"),
		Limit:    queryInt(r, "limit", 20),
		Offset:   queryInt(r, "offset", 0),
	}
	items, err := s.store.ListWorks(r.Context(), f)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writePage(w, items, f.Offset, f.Limit)
}

func (s *Server) myWorks(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	f := store.WorkFilter{
		ViewerID: id.UserID, OwnerID: id.UserID,
		Status: r.URL.Query().Get("status"),
		Limit:  queryInt(r, "limit", 30), Offset: queryInt(r, "offset", 0),
	}
	items, err := s.store.ListWorks(r.Context(), f)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writePage(w, items, f.Offset, f.Limit)
}

// importWork 把外部音乐引擎生成的作品登记到主后端，使其出现在「我的作品」。
// 请求为 multipart/form-data：title、genre、tags、duration_seconds、description 和音频文件 audio。
func (s *Server) importWork(w http.ResponseWriter, r *http.Request) {
	me, okUser := currentUser(w, r)
	if !okUser {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid multipart form")
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "title is required")
		return
	}
	file, _, err := r.FormFile("audio")
	if err != nil {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "audio file is required")
		return
	}
	defer file.Close()

	workID := store.NewID("w_")
	dir := filepath.Join(s.cfg.MediaDir, "generated")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	dstPath := filepath.Join(dir, workID+".wav")
	dst, err := os.Create(dstPath)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if err := dst.Close(); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	duration := 30
	if v, convErr := strconv.Atoi(r.FormValue("duration_seconds")); convErr == nil && v > 0 && v <= 600 {
		duration = v
	}
	genre := strings.TrimSpace(r.FormValue("genre"))
	description := strings.TrimSpace(r.FormValue("description"))
	tags := importTags(r.FormValue("tags"))
	if len(tags) == 0 && genre != "" {
		tags = []string{genre}
	}

	decision, reason := core.Moderate(title, description)
	status := "published"
	var publishedAt *time.Time
	if decision == "reject" {
		status = "rejected"
	} else {
		now := time.Now()
		publishedAt = &now
	}
	createdID, err := s.store.CreateWork(r.Context(), store.CreateWorkInput{
		TenantID: me.TenantID, UserID: me.UserID, Title: title, Cover: "c1",
		Description: description, Visibility: "public", Status: status,
		RejectReason: reason, Genre: genre, Tags: tags,
		AudioURL: s.core.PublishAudio(r.Context(), "music/generated/"+workID+".wav", dstPath),
		Duration: duration, IsAI: true, PublishedAt: publishedAt,
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	work, err := s.store.GetWork(r.Context(), createdID, me.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, work)
}

func importTags(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '/' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func writePage(w http.ResponseWriter, items any, offset, limit int) {
	count := 0
	v := reflect.ValueOf(items)
	if v.IsValid() && (v.Kind() == reflect.Slice || v.Kind() == reflect.Array) {
		count = v.Len()
	}
	ok(w, map[string]any{
		"items":       items,
		"next_cursor": strconv.Itoa(offset + count),
		"has_more":    count == limit,
	})
}

func (s *Server) getWork(w http.ResponseWriter, r *http.Request) {
	viewer, _ := identity(r)
	work, err := s.store.GetWork(r.Context(), pathID(r, "id"), viewer.UserID)
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "work not found")
		return
	}
	ok(w, work)
}

func (s *Server) deleteWork(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	if err := s.store.RemoveWork(r.Context(), pathID(r, "id"), id.UserID); err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "work not found")
		return
	}
	ok(w, map[string]any{"deleted": true})
}

func (s *Server) setVisibility(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		Visibility string `json:"visibility"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Visibility != "public" && req.Visibility != "friends" && req.Visibility != "private" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid visibility")
		return
	}
	if err := s.store.SetVisibility(r.Context(), pathID(r, "id"), id.UserID, req.Visibility); err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "work not found")
		return
	}
	ok(w, map[string]any{"visibility": req.Visibility})
}

func (s *Server) publishWork(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req core.PublishRequest
	if !decode(w, r, &req) {
		return
	}
	work, err := s.core.Publish(r.Context(), pathID(r, "id"), id.UserID, req)
	if err != nil {
		fail(w, http.StatusBadRequest, "INVALID_STATE", err.Error())
		return
	}
	ok(w, work)
}

func (s *Server) likeWork(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	workID := pathID(r, "id")
	work, err := s.store.GetWork(r.Context(), workID, id.UserID)
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "work not found")
		return
	}
	count, err := s.store.Like(r.Context(), workID, id.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if work.OwnerID != id.UserID {
		_ = s.store.Notify(r.Context(), work.OwnerID, "interaction", id.UserID, "有人赞了你的作品", "《"+work.Title+"》", "work", workID, "like:"+workID)
	}
	ok(w, map[string]any{"liked": true, "like_count": count})
}

func (s *Server) unlikeWork(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	count, err := s.store.Unlike(r.Context(), pathID(r, "id"), id.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"liked": false, "like_count": count})
}

func (s *Server) collectWork(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	if _, err := s.store.GetWork(r.Context(), pathID(r, "id"), id.UserID); err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "work not found")
		return
	}
	if err := s.store.Collect(r.Context(), pathID(r, "id"), id.UserID); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"collected": true})
}

func (s *Server) uncollectWork(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	if err := s.store.Uncollect(r.Context(), pathID(r, "id"), id.UserID); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"collected": false})
}

func (s *Server) playWork(w http.ResponseWriter, r *http.Request) {
	if err := s.store.IncrementPlay(r.Context(), pathID(r, "id")); err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "work not found")
		return
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListComments(r.Context(), pathID(r, "id"), queryInt(r, "limit", 50))
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) addComment(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" || len([]rune(req.Content)) > 200 {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "评论长度需为 1–200 字")
		return
	}
	workID := pathID(r, "id")
	work, err := s.store.GetWork(r.Context(), workID, id.UserID)
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "work not found")
		return
	}
	c, err := s.store.AddComment(r.Context(), workID, id.UserID, req.Content)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if work.OwnerID != id.UserID {
		_ = s.store.Notify(r.Context(), work.OwnerID, "interaction", id.UserID, "有人评论了你的作品", req.Content, "work", workID, "comment:"+workID)
	}
	ok(w, c)
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	if err := s.store.DeleteComment(r.Context(), pathID(r, "id"), id.UserID); err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "comment not found")
		return
	}
	ok(w, map[string]any{"deleted": true})
}

/* ---------------- feeds ---------------- */

func (s *Server) recommend(w http.ResponseWriter, r *http.Request) {
	viewer, _ := identity(r)
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	items, err := s.store.ListWorks(r.Context(), store.WorkFilter{
		ViewerID: viewer.UserID, Sort: "hot", Limit: limit, Offset: offset,
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	following, _ := s.store.FollowingIDs(r.Context(), viewer.UserID)
	set := map[string]bool{}
	for _, id := range following {
		set[id] = true
	}
	for i := range items {
		if set[items[i].OwnerID] {
			items[i].Reason = "因为你关注了 " + items[i].OwnerName
		} else {
			items[i].Reason = "为你推荐"
		}
	}
	// 推荐流不足时用运营配置的保底内容补齐（仅首页）。
	if offset == 0 && len(items) < limit {
		ids := make([]string, 0, len(items))
		for _, w := range items {
			ids = append(ids, w.ID)
		}
		if fb, err := s.store.FallbackWorks(r.Context(), viewer.UserID, ids, limit-len(items)); err == nil {
			for i := range fb {
				fb[i].Reason = "编辑精选"
			}
			items = append(items, fb...)
		}
	}
	version := ""
	if p, err := s.store.ActiveFeedPolicy(r.Context()); err == nil {
		version = p.Version
	}
	ok(w, map[string]any{
		"items": items, "policy_version": version,
		"next_cursor": strconv.Itoa(offset + limit), "has_more": len(items) == limit,
	})
}

func (s *Server) followingFeed(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	ids, err := s.store.FollowingIDs(r.Context(), id.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	set := map[string]bool{}
	for _, x := range ids {
		set[x] = true
	}
	limit := queryInt(r, "limit", 20)
	items, err := s.store.ListWorks(r.Context(), store.WorkFilter{
		ViewerID: id.UserID, Limit: 100, Offset: queryInt(r, "offset", 0),
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	out := []any{}
	for i := range items {
		if set[items[i].OwnerID] {
			items[i].Reason = "来自你关注的 " + items[i].OwnerName
			out = append(out, items[i])
			if len(out) >= limit {
				break
			}
		}
	}
	ok(w, map[string]any{"items": out, "has_more": false})
}

func (s *Server) plaza(w http.ResponseWriter, r *http.Request) {
	viewer, _ := identity(r)
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	items, err := s.store.ListWorks(r.Context(), store.WorkFilter{
		ViewerID: viewer.UserID,
		Sort:     r.URL.Query().Get("sort"),
		Genre:    r.URL.Query().Get("genre"),
		Limit:    limit, Offset: offset,
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{
		"items": items, "next_cursor": strconv.Itoa(offset + limit), "has_more": len(items) == limit,
	})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	viewer, _ := identity(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		ok(w, map[string]any{"works": []any{}, "artists": []any{}})
		return
	}
	works, err := s.store.ListWorks(r.Context(), store.WorkFilter{
		ViewerID: viewer.UserID, Query: q, Limit: 20,
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	artists, err := s.store.SearchUsers(r.Context(), viewer.UserID, q, 10)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"works": works, "artists": artists})
}

func (s *Server) feedFeedback(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		Type       string `json:"type"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.TargetType == "" || req.TargetID == "" || req.Type == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "target and type are required")
		return
	}
	if err := s.store.AddFeedback(r.Context(), id.UserID, req.TargetType, req.TargetID, req.Type); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

/* ---------------- credits ---------------- */

func (s *Server) createQuote(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req core.QuoteRequest
	if !decode(w, r, &req) {
		return
	}
	q, err := s.core.CreateQuote(r.Context(), id.TenantID, id.UserID, req)
	if err != nil {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	ok(w, q)
}

func (s *Server) packages(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Packages(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) recharge(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		PackageID string `json:"package_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.store.Recharge(r.Context(), id.TenantID, id.UserID, req.PackageID); err != nil {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	acct, _ := s.store.Account(r.Context(), id.TenantID)
	ok(w, acct)
}

func (s *Server) credits(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	acct, err := s.store.Account(r.Context(), id.TenantID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, acct)
}

func (s *Server) ledger(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	items, err := s.store.Ledger(r.Context(), id.TenantID, queryInt(r, "limit", 30))
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

/* ---------------- generations ---------------- */

func (s *Server) createGeneration(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req core.GenerateRequest
	if !decode(w, r, &req) {
		return
	}
	gen, err := s.core.CreateGeneration(r.Context(), id.TenantID, id.UserID, req)
	if err != nil {
		var creditErr *core.InsufficientCreditError
		if errors.As(err, &creditErr) {
			write(w, http.StatusPaymentRequired, map[string]any{
				"code":    "INSUFFICIENT_CREDIT",
				"message": creditErr.Error(),
				"data":    map[string]any{"required": creditErr.Required, "available": creditErr.Available},
			})
			return
		}
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	ok(w, gen)
}

func (s *Server) getGeneration(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	gen, err := s.store.GetGeneration(r.Context(), pathID(r, "id"), id.UserID)
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "generation not found")
		return
	}
	ok(w, gen)
}

func (s *Server) cancelGeneration(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	gen, err := s.core.Cancel(r.Context(), pathID(r, "id"), id.UserID)
	if err != nil {
		fail(w, http.StatusConflict, "INVALID_STATE", err.Error())
		return
	}
	ok(w, gen)
}

func (s *Server) generationEvents(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	flusher, canFlush := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			gen, err := s.store.GetGeneration(r.Context(), pathID(r, "id"), id.UserID)
			if err != nil {
				return
			}
			b, _ := json.Marshal(gen)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
			switch gen.Status {
			case "succeeded", "failed", "cancelled":
				return
			}
		}
	}
}

/* ---------------- share ---------------- */

func (s *Server) createShareLink(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	var req struct {
		WorkID string `json:"work_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	work, err := s.store.GetWorkAny(r.Context(), req.WorkID)
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "work not found")
		return
	}
	if work.Status != "published" || work.Visibility != "public" {
		fail(w, http.StatusForbidden, "FORBIDDEN", "仅公开作品可生成站外分享链接")
		return
	}
	code := store.NewID("")
	if len(code) > 10 {
		code = code[:10]
	}
	if err := s.store.CreateShareLink(r.Context(), req.WorkID, id.UserID, code); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	// 分享成功即发放奖励：同一作品每天只奖励一次，重复分享返回 0。
	reward, err := s.store.RewardShare(r.Context(), id.TenantID, id.UserID, work.ID, localDate())
	if err != nil {
		slog.Error("share reward failed", "user", id.UserID, "work", work.ID, "error", err)
		reward = 0
	}
	ok(w, map[string]any{
		"code": code, "url": "/api/v1/s/" + code,
		"reward_credits": reward, "rewarded": reward > 0,
	})
}

func (s *Server) shareLanding(w http.ResponseWriter, r *http.Request) {
	link, err := s.store.GetShareLink(r.Context(), pathID(r, "code"))
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "share link not found")
		return
	}
	work, err := s.store.GetWorkAny(r.Context(), link.WorkID)
	if err != nil || work.Status != "published" || work.Visibility != "public" {
		fail(w, http.StatusGone, "CONTENT_UNAVAILABLE", "内容不可用")
		return
	}
	ok(w, map[string]any{"work": work, "sharer_id": link.SharerID, "code": link.Code})
}

func (s *Server) shareEvent(w http.ResponseWriter, r *http.Request) {
	id, _ := identity(r)
	var req struct {
		Code      string `json:"code"`
		Channel   string `json:"channel"`
		EventType string `json:"event_type"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Code == "" || req.EventType == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "code and event_type are required")
		return
	}
	if err := s.store.AddShareEvent(r.Context(), req.Code, req.Channel, req.EventType, id.UserID); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}
