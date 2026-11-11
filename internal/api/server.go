package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/pathvar"

	"github.com/jackmengtag/SonicHub_api/internal/config"
	"github.com/jackmengtag/SonicHub_api/internal/core"
	"github.com/jackmengtag/SonicHub_api/internal/platform/auth"
	"github.com/jackmengtag/SonicHub_api/internal/platform/dysms"
	"github.com/jackmengtag/SonicHub_api/internal/platform/pns"
	"github.com/jackmengtag/SonicHub_api/internal/platform/rbac"
	"github.com/jackmengtag/SonicHub_api/internal/store"
)

type Server struct {
	cfg   config.Config
	store *store.Store
	core  *core.Core
	pns   *pns.Client
	dysms *dysms.Client
}

func New(cfg config.Config, st *store.Store, c *core.Core) *Server {
	pnsClient, err := pns.New(cfg)
	if err != nil {
		slog.Error("pns client init failed", "error", err)
	}
	dysmsClient, err := dysms.New(cfg)
	if err != nil {
		slog.Error("dysms client init failed", "error", err)
	}
	slog.Info("sms provider",
		"provider", cfg.SMSProvider,
		"access_key_configured", pnsClient != nil && pnsClient.Available(),
		"sms_enabled", pnsClient != nil && pnsClient.Enabled(),
		"dysms_enabled", dysmsClient != nil && dysmsClient.Enabled())
	return &Server{cfg: cfg, store: st, core: c, pns: pnsClient, dysms: dysmsClient}
}

func (s *Server) Start() {
	host, port, err := net.SplitHostPort(s.cfg.Addr)
	if err != nil {
		host, port = "", "8080"
	}
	conf := rest.RestConf{
		ServiceConf: service.ServiceConf{Name: "sonichub-api"},
		Host:        host,
		Port:        mustInt(port),
		Timeout:     30000,
		MaxConns:    20000,
	}
	server := rest.MustNewServer(conf, rest.WithCors("*"))
	defer server.Stop()
	server.Use(s.middleware)
	s.routes(server)
	slog.Info("http server listening", "addr", s.cfg.Addr)
	server.Start()
}

func mustInt(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 8080
		}
		n = n*10 + int(r-'0')
	}
	if n == 0 {
		return 8080
	}
	return n
}

func (s *Server) routes(server *rest.Server) {
	add := func(method, path string, h http.HandlerFunc) {
		server.AddRoute(rest.Route{Method: method, Path: path, Handler: h})
	}
	addPerm := func(perm, method, path string, h http.HandlerFunc) {
		server.AddRoute(rest.Route{
			Method: method,
			Path:   path,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				if _, ok := requirePermission(w, r, perm); !ok {
					return
				}
				h(w, r)
			},
		})
	}

	add(http.MethodGet, "/", s.debugConsole)
	add(http.MethodGet, "/health", s.health)
	add(http.MethodGet, "/media/generated/:name", s.media)

	add(http.MethodPost, "/api/v1/auth/login", s.login)
	add(http.MethodPost, "/api/v1/auth/sms/send", s.sendSMSCode)
	add(http.MethodPost, "/api/v1/auth/sms/tokens", s.smsSDKTokens)
	add(http.MethodPost, "/api/v1/auth/sms/verify", s.smsSDKVerify)
	add(http.MethodPost, "/api/v1/auth/one-click/login", s.oneClickLogin)
	add(http.MethodPost, "/api/v1/auth/one-click/verify", s.verifyMobile)
	add(http.MethodPost, "/api/v1/auth/refresh", s.refresh)
	add(http.MethodPost, "/api/v1/auth/logout", s.logout)
	add(http.MethodGet, "/api/v1/me", s.me)
	add(http.MethodPut, "/api/v1/me", s.updateMe)
	add(http.MethodPost, "/api/v1/me/interests", s.saveInterests)

	add(http.MethodGet, "/api/v1/users/:id", s.userProfile)
	add(http.MethodPost, "/api/v1/users/:id/follow", s.follow)
	add(http.MethodDelete, "/api/v1/users/:id/follow", s.unfollow)
	add(http.MethodGet, "/api/v1/users/:id/following", s.userFollowing)
	add(http.MethodGet, "/api/v1/users/:id/followers", s.userFollowers)
	add(http.MethodPost, "/api/v1/users/:id/block", s.blockUser)
	add(http.MethodDelete, "/api/v1/users/:id/block", s.unblockUser)

	add(http.MethodGet, "/api/v1/me/following", s.meFollowing)
	add(http.MethodGet, "/api/v1/me/followers", s.meFollowers)
	add(http.MethodGet, "/api/v1/me/friends", s.meFriends)
	add(http.MethodGet, "/api/v1/me/works", s.myWorks)

	add(http.MethodGet, "/api/v1/musicians/:id", s.musicianProfile)
	add(http.MethodPost, "/api/v1/musicians/applications", s.applyMusician)
	add(http.MethodGet, "/api/v1/musicians/applications/me", s.myApplication)

	add(http.MethodGet, "/api/v1/works", s.listWorks)
	add(http.MethodGet, "/api/v1/works/:id", s.getWork)
	add(http.MethodDelete, "/api/v1/works/:id", s.deleteWork)
	add(http.MethodPut, "/api/v1/works/:id/visibility", s.setVisibility)
	add(http.MethodPost, "/api/v1/works/:id/publish", s.publishWork)
	add(http.MethodPost, "/api/v1/works/import", s.importWork)
	add(http.MethodPost, "/api/v1/works/:id/like", s.likeWork)
	add(http.MethodDelete, "/api/v1/works/:id/like", s.unlikeWork)
	add(http.MethodPost, "/api/v1/works/:id/collect", s.collectWork)
	add(http.MethodDelete, "/api/v1/works/:id/collect", s.uncollectWork)
	add(http.MethodPost, "/api/v1/works/:id/play", s.playWork)
	add(http.MethodGet, "/api/v1/works/:id/comments", s.listComments)
	add(http.MethodPost, "/api/v1/works/:id/comments", s.addComment)
	add(http.MethodDelete, "/api/v1/comments/:id", s.deleteComment)

	add(http.MethodGet, "/api/v1/feed/recommend", s.recommend)
	add(http.MethodGet, "/api/v1/feed/following", s.followingFeed)
	add(http.MethodPost, "/api/v1/feed/feedback", s.feedFeedback)
	add(http.MethodGet, "/api/v1/plaza", s.plaza)
	add(http.MethodGet, "/api/v1/search", s.search)

	add(http.MethodPost, "/api/v1/credit/quotes", s.createQuote)
	add(http.MethodGet, "/api/v1/credit/packages", s.packages)
	add(http.MethodPost, "/api/v1/credit/recharge", s.recharge)
	add(http.MethodGet, "/api/v1/me/credits", s.credits)
	add(http.MethodGet, "/api/v1/me/credits/ledger", s.ledger)
	add(http.MethodPost, "/api/v1/checkins", s.checkin)
	add(http.MethodGet, "/api/v1/checkins/status", s.checkinStatus)

	add(http.MethodPost, "/api/v1/generations", s.createGeneration)
	add(http.MethodGet, "/api/v1/generations/:id", s.getGeneration)
	add(http.MethodPost, "/api/v1/generations/:id/cancel", s.cancelGeneration)
	add(http.MethodGet, "/api/v1/generations/:id/events", s.generationEvents)

	add(http.MethodGet, "/api/v1/notifications", s.notifications)
	add(http.MethodPost, "/api/v1/notifications/read", s.readNotifications)
	add(http.MethodPost, "/api/v1/notifications/:id/read", s.readNotification)

	add(http.MethodGet, "/api/v1/conversations", s.conversations)
	add(http.MethodPost, "/api/v1/conversations", s.createConversation)
	add(http.MethodGet, "/api/v1/conversations/:id/messages", s.messages)
	add(http.MethodPost, "/api/v1/conversations/:id/messages", s.sendMessage)
	add(http.MethodPost, "/api/v1/conversations/:id/read", s.readConversation)

	add(http.MethodPost, "/api/v1/share-links", s.createShareLink)
	add(http.MethodGet, "/api/v1/s/:code", s.shareLanding)
	add(http.MethodPost, "/api/v1/share-events", s.shareEvent)

	add(http.MethodPost, "/api/v1/reports", s.createReport)
	add(http.MethodPost, "/api/v1/appeals", s.createAppeal)
	add(http.MethodGet, "/api/v1/me/appeals", s.myAppeals)

	add(http.MethodGet, "/api/v1/admin/roles", s.adminRoles)
	addPerm("dashboard.view", http.MethodGet, "/api/v1/admin/stats", s.adminStats)
	addPerm("moderation.review", http.MethodGet, "/api/v1/admin/moderation", s.adminModeration)
	addPerm("moderation.review", http.MethodPost, "/api/v1/admin/moderation/:id/decision", s.adminDecideWork)
	addPerm("moderation.review", http.MethodGet, "/api/v1/admin/reports", s.adminReports)
	addPerm("moderation.review", http.MethodPost, "/api/v1/admin/reports/:id/decision", s.adminDecideReport)
	addPerm("creators.review", http.MethodGet, "/api/v1/admin/applications", s.adminApplications)
	addPerm("creators.review", http.MethodPost, "/api/v1/admin/applications/:id/decision", s.adminDecideApplication)
	addPerm("moderation.review", http.MethodGet, "/api/v1/admin/appeals", s.adminAppeals)
	addPerm("moderation.review", http.MethodPost, "/api/v1/admin/appeals/:id/decision", s.adminDecideAppeal)
	addPerm("recommend.publish", http.MethodGet, "/api/v1/admin/feed-policy", s.adminGetPolicy)
	addPerm("recommend.publish", http.MethodPut, "/api/v1/admin/feed-policy", s.adminSavePolicy)
	addPerm("recommend.publish", http.MethodGet, "/api/v1/admin/feed-lists", s.adminFeedLists)
	addPerm("recommend.publish", http.MethodPost, "/api/v1/admin/feed-lists", s.adminCreateFeedList)
	addPerm("recommend.publish", http.MethodDelete, "/api/v1/admin/feed-lists/:id", s.adminDeleteFeedList)
	addPerm("recommend.publish", http.MethodGet, "/api/v1/admin/feed-fallbacks", s.adminFeedFallbacks)
	addPerm("recommend.publish", http.MethodPost, "/api/v1/admin/feed-fallbacks", s.adminCreateFeedFallback)
	addPerm("recommend.publish", http.MethodDelete, "/api/v1/admin/feed-fallbacks/:id", s.adminDeleteFeedFallback)

	addPerm("users.manage", http.MethodGet, "/api/v1/admin/users", s.adminUsers)
	addPerm("users.manage", http.MethodGet, "/api/v1/admin/users/:id", s.adminUserDetail)
	addPerm("users.manage", http.MethodPost, "/api/v1/admin/users/:id/decision", s.adminUserDecision)
	addPerm("roles.manage", http.MethodPut, "/api/v1/admin/users/:id/role", s.adminSetUserRole)

	addPerm("works.manage", http.MethodGet, "/api/v1/admin/works", s.adminWorks)
	addPerm("works.manage", http.MethodGet, "/api/v1/admin/works/:id", s.adminWorkDetail)
	addPerm("works.manage", http.MethodPost, "/api/v1/admin/works/:id/decision", s.adminWorkDecision)

	addPerm("credits.manage", http.MethodGet, "/api/v1/admin/credits/summary", s.adminCreditSummary)
	addPerm("credits.manage", http.MethodGet, "/api/v1/admin/credits/ledger", s.adminCreditLedger)
	addPerm("credits.manage", http.MethodGet, "/api/v1/admin/credits/adjustments", s.adminCreditAdjustments)
	addPerm("credits.manage", http.MethodPost, "/api/v1/admin/credits/adjustments", s.adminCreateCreditAdjustment)
	addPerm("credits.review", http.MethodPost, "/api/v1/admin/credits/adjustments/:id/decision", s.adminDecideCreditAdjustment)

	addPerm("messages.manage", http.MethodGet, "/api/v1/admin/messages", s.adminMessages)
	addPerm("messages.manage", http.MethodPost, "/api/v1/admin/messages", s.adminCreateMessage)
	addPerm("messages.manage", http.MethodPost, "/api/v1/admin/messages/:id/send", s.adminSendMessage)
	addPerm("messages.manage", http.MethodGet, "/api/v1/admin/message-templates", s.adminTemplates)
	addPerm("messages.manage", http.MethodPost, "/api/v1/admin/message-templates", s.adminCreateTemplate)
	addPerm("messages.manage", http.MethodPut, "/api/v1/admin/message-templates/:id", s.adminUpdateTemplate)

	addPerm("plaza.manage", http.MethodGet, "/api/v1/admin/plaza/placements", s.adminPlacements)
	addPerm("plaza.manage", http.MethodPost, "/api/v1/admin/plaza/placements", s.adminCreatePlacement)
	addPerm("plaza.manage", http.MethodPut, "/api/v1/admin/plaza/placements/:id", s.adminUpdatePlacement)
	addPerm("plaza.manage", http.MethodGet, "/api/v1/admin/plaza/topics", s.adminTopics)
	addPerm("plaza.manage", http.MethodPost, "/api/v1/admin/plaza/topics", s.adminCreateTopic)
	addPerm("plaza.manage", http.MethodPut, "/api/v1/admin/plaza/topics/:id", s.adminUpdateTopic)
	addPerm("plaza.manage", http.MethodGet, "/api/v1/admin/plaza/ranking", s.adminRanking)

	addPerm("audit.view", http.MethodGet, "/api/v1/admin/audit-logs", s.adminAuditLogs)
	addPerm("audit.view", http.MethodGet, "/api/v1/admin/audit-logs/:id", s.adminAuditLogDetail)
}

/* ---------------- middleware ---------------- */

func (s *Server) middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Tenant-ID,Idempotency-Key,X-Request-ID")
		w.Header().Set("X-Request-ID", uuid.NewString())
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic", "error", rec, "path", r.URL.Path)
				fail(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
			}
		}()

		public := isPublic(r.Method, r.URL.Path)
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if header == "" {
			if public {
				next(w, r)
				return
			}
			fail(w, http.StatusUnauthorized, "UNAUTHENTICATED", "authorization required")
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		id, err := auth.ParseAccessToken(s.cfg.JWTSecret, token)
		if err != nil {
			fail(w, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid token")
			return
		}
		next(w, r.WithContext(auth.WithIdentity(r.Context(), *id)))
	}
}

func isPublic(method, path string) bool {
	if path == "/" || path == "/health" || strings.HasPrefix(path, "/media/") {
		return true
	}
	if method == http.MethodPost && (path == "/api/v1/auth/login" || path == "/api/v1/auth/sms/send" ||
		path == "/api/v1/auth/sms/tokens" || path == "/api/v1/auth/sms/verify" ||
		path == "/api/v1/auth/one-click/login" || path == "/api/v1/auth/one-click/verify" ||
		path == "/api/v1/auth/refresh") {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/s/") {
		return true
	}
	if method == http.MethodGet {
		switch path {
		case "/api/v1/plaza", "/api/v1/search", "/api/v1/feed/recommend",
			"/api/v1/works", "/api/v1/credit/packages":
			return true
		}
		if strings.HasPrefix(path, "/api/v1/works/") || strings.HasPrefix(path, "/api/v1/musicians/") {
			return true
		}
	}
	return false
}

/* ---------------- helpers ---------------- */

func identity(r *http.Request) (auth.Identity, bool) {
	return auth.FromContext(r.Context())
}

func currentUser(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	id, ok := identity(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "UNAUTHENTICATED", "authorization required")
		return id, false
	}
	return id, true
}

func requireStaff(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	id, ok := currentUser(w, r)
	if !ok {
		return id, false
	}
	if !rbac.IsStaff(id.Role) {
		fail(w, http.StatusForbidden, "FORBIDDEN", "admin permission required")
		return id, false
	}
	return id, true
}

func requirePermission(w http.ResponseWriter, r *http.Request, perm string) (auth.Identity, bool) {
	id, ok := requireStaff(w, r)
	if !ok {
		return id, false
	}
	if !rbac.Allows(id.Role, perm) {
		fail(w, http.StatusForbidden, "FORBIDDEN", "缺少权限："+perm)
		return id, false
	}
	return id, true
}

func pathID(r *http.Request, key string) string {
	return pathvar.Vars(r)[key]
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid JSON body")
		return false
	}
	return true
}

func ok(w http.ResponseWriter, data any) {
	write(w, http.StatusOK, map[string]any{"code": "OK", "message": "ok", "data": data})
}

func fail(w http.ResponseWriter, status int, code, message string) {
	write(w, status, map[string]any{"code": code, "message": message})
}

func write(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		fail(w, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "database unavailable")
		return
	}
	ok(w, map[string]any{"status": "ok", "service": "sonichub-api"})
}

func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	name := pathID(r, "name")
	if strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.cfg.MediaDir, "generated", name))
}
