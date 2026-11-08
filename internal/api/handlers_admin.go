package api

import "net/http"

func (s *Server) adminStats(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	stats, err := s.store.AdminStats(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, stats)
}

func (s *Server) adminModeration(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.PendingWorks(r.Context(), queryInt(r, "limit", 30))
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) adminDecideWork(w http.ResponseWriter, r *http.Request) {
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
	owner, err := s.store.DecideWork(r.Context(), pathID(r, "id"), id.UserID, req.Decision, req.Reason)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	title := "作品审核通过"
	body := "你的作品已通过审核"
	if req.Decision == "reject" {
		title = "作品未通过审核"
		body = req.Reason
	}
	_ = s.store.Notify(r.Context(), owner, "work", "", title, body, "work", pathID(r, "id"), "moderation:"+pathID(r, "id"))
	ok(w, map[string]any{"ok": true})
}

func (s *Server) adminReports(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListReports(r.Context(), queryInt(r, "limit", 30))
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) adminDecideReport(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Decision string `json:"decision"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Decision == "" {
		req.Decision = "dismissed"
	}
	if err := s.store.DecideReport(r.Context(), pathID(r, "id"), id.UserID, req.Decision); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) adminApplications(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListApplications(r.Context(), queryInt(r, "limit", 30))
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) adminDecideApplication(w http.ResponseWriter, r *http.Request) {
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
	userID := pathID(r, "id")
	if err := s.store.DecideApplication(r.Context(), userID, id.UserID, req.Decision, req.Reason); err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	title := "音乐人认证通过"
	body := "你已获得音乐人认证标识"
	if req.Decision == "reject" {
		title = "音乐人认证未通过"
		body = req.Reason
	}
	_ = s.store.Notify(r.Context(), userID, "system", "", title, body, "musician", userID, "musician:"+userID)
	ok(w, map[string]any{"ok": true})
}

func (s *Server) adminAppeals(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	items, err := s.store.ListAppeals(r.Context(), queryInt(r, "limit", 30))
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, map[string]any{"items": items})
}

func (s *Server) adminDecideAppeal(w http.ResponseWriter, r *http.Request) {
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
	if req.Decision != "accept" && req.Decision != "reject" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "decision must be accept or reject")
		return
	}
	owner, err := s.store.DecideAppeal(r.Context(), pathID(r, "id"), id.UserID, req.Decision, req.Reason)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	title := "申诉已受理"
	body := "你的作品已恢复发布"
	if req.Decision == "reject" {
		title = "申诉未通过"
		body = req.Reason
	}
	_ = s.store.Notify(r.Context(), owner, "work", "", title, body, "appeal", pathID(r, "id"), "appeal:"+pathID(r, "id"))
	ok(w, map[string]any{"ok": true})
}

func (s *Server) adminGetPolicy(w http.ResponseWriter, r *http.Request) {
	if _, okID := requireStaff(w, r); !okID {
		return
	}
	p, err := s.store.ActiveFeedPolicy(r.Context())
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "feed policy not found")
		return
	}
	ok(w, p)
}

func (s *Server) adminSavePolicy(w http.ResponseWriter, r *http.Request) {
	id, okID := requireStaff(w, r)
	if !okID {
		return
	}
	var req struct {
		Version   string         `json:"version"`
		Weights   map[string]any `json:"weights"`
		Diversity map[string]any `json:"diversity"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Version == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "version is required")
		return
	}
	if req.Weights == nil {
		req.Weights = map[string]any{}
	}
	if req.Diversity == nil {
		req.Diversity = map[string]any{}
	}
	p, err := s.store.SaveFeedPolicy(r.Context(), id.UserID, req.Version, req.Weights, req.Diversity)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, p)
}
