package api

import (
	"net/http"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/store"
)

// 积分相关的自然日（签到、分享奖励）按北京时间计算，避免部署环境时区（如容器默认 UTC）导致跨天错位。
var checkinZone = time.FixedZone("CST", 8*60*60)

func localDate() string {
	return time.Now().In(checkinZone).Format("2006-01-02")
}

func (s *Server) checkin(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	result, err := s.store.Checkin(r.Context(), id.TenantID, id.UserID, localDate(), store.DailyCheckinCredits)
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, result)
}

func (s *Server) checkinStatus(w http.ResponseWriter, r *http.Request) {
	id, okID := currentUser(w, r)
	if !okID {
		return
	}
	status, err := s.store.CheckinStatus(r.Context(), id.TenantID, id.UserID, localDate())
	if err != nil {
		fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	ok(w, status)
}
