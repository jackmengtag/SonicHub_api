package api

import (
	"net/http"
	"strings"

	"github.com/jackmengtag/SonicHub_api/internal/platform/rbac"
)

func (s *Server) adminRoles(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	ok(w, map[string]any{
		"roles":          rbac.Roles(),
		"permissions":    rbac.Permissions(),
		"matrix":         rbac.Matrix(),
		"my_role":        id.Role,
		"my_role_name":   rbac.RoleName(id.Role),
		"my_permissions": rbac.PermissionsFor(id.Role),
	})
}

func (s *Server) adminSetUserRole(w http.ResponseWriter, r *http.Request) {
	actor, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	if !rbac.IsKnown(req.Role) {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "未知角色："+req.Role)
		return
	}
	userID := pathID(r, "id")
	old, err := s.store.SetUserRole(r.Context(), userID, actor.UserID, req.Role)
	if err != nil {
		failStore(w, err)
		return
	}
	if old != req.Role {
		_ = s.store.Notify(r.Context(), userID, "system", "", "后台角色已调整",
			"你的后台角色已调整为："+rbac.RoleName(req.Role)+"，重新登录后生效。", "user", userID, "role:"+userID)
	}
	ok(w, map[string]any{"ok": true, "old_role": old, "role": req.Role, "role_name": rbac.RoleName(req.Role)})
}

/* ---------------- 推荐黑白名单与保底内容 ---------------- */

func (s *Server) adminFeedLists(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListFeedLists(r.Context(), "")
	if err != nil {
		failStore(w, err)
		return
	}
	black, white := []string{}, []string{}
	for _, item := range items {
		if item.ListType == "black" {
			black = append(black, item.Value)
		} else if item.ListType == "white" {
			white = append(white, item.Value)
		}
	}
	ok(w, map[string]any{"items": items, "black": black, "white": white})
}

func (s *Server) adminCreateFeedList(w http.ResponseWriter, r *http.Request) {
	actor, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		ListType string `json:"list_type"`
		Value    string `json:"value"`
	}
	if !decode(w, r, &req) {
		return
	}
	entry, err := s.store.CreateFeedList(r.Context(), actor.UserID, req.ListType, req.Value)
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, entry)
}

func (s *Server) adminDeleteFeedList(w http.ResponseWriter, r *http.Request) {
	actor, okID := requireStaff(w, r)
	if !okID {
		return
	}
	if err := s.store.DeleteFeedList(r.Context(), actor.UserID, pathID(r, "id")); err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) adminFeedFallbacks(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListFeedFallbacks(r.Context())
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) adminCreateFeedFallback(w http.ResponseWriter, r *http.Request) {
	actor, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		WorkID string `json:"work_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	item, err := s.store.CreateFeedFallback(r.Context(), actor.UserID, req.WorkID)
	if err != nil {
		failStore(w, err)
		return
	}
	ok(w, item)
}

func (s *Server) adminDeleteFeedFallback(w http.ResponseWriter, r *http.Request) {
	actor, okID := requireStaff(w, r)
	if !okID {
		return
	}
	if err := s.store.DeleteFeedFallback(r.Context(), actor.UserID, pathID(r, "id")); err != nil {
		failStore(w, err)
		return
	}
	ok(w, map[string]any{"ok": true})
}
