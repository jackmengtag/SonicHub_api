package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackmengtag/SonicHub_api/internal/core"
	"github.com/jackmengtag/SonicHub_api/internal/platform/music"
)

// mobileLyrics 供手机端（Android / iOS）调用的 AI 写词接口。
// 请求体：{"theme":"夏天的海边","genre":"pop","mood":"happy","bpm":90}
// 成功返回：{"lyrics":"[Verse 1]...","genre":"pop","mood":"happy","bpm":90}
func (s *Server) mobileLyrics(w http.ResponseWriter, r *http.Request) {
	if _, okUser := currentUser(w, r); !okUser {
		return
	}
	if s.core.MusicModel == nil || !s.core.MusicModel.Enabled() {
		fail(w, http.StatusServiceUnavailable, "MODEL_UNAVAILABLE", "音乐大模型未配置，暂不可用")
		return
	}

	var req struct {
		Theme  string `json:"theme"`
		Prompt string `json:"prompt"`
		Genre  string `json:"genre"`
		Mood   string `json:"mood"`
		BPM    int    `json:"bpm"`
	}
	if !decode(w, r, &req) {
		return
	}
	theme := strings.TrimSpace(req.Theme)
	if theme == "" {
		theme = strings.TrimSpace(req.Prompt)
	}
	if theme == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "theme is required")
		return
	}

	lyrics, err := s.core.MusicModel.GenerateLyrics(r.Context(), music.LyricsRequest{
		Theme: theme, Genre: req.Genre, Mood: req.Mood, BPM: req.BPM,
	})
	if err != nil {
		fail(w, http.StatusBadGateway, "LYRICS_GENERATE_FAILED", err.Error())
		return
	}
	ok(w, map[string]any{
		"lyrics": lyrics,
		"genre":  defaultArg(req.Genre, "pop"),
		"mood":   defaultArg(req.Mood, "peaceful"),
		"bpm":    req.BPM,
	})
}

// mobileMusic 供手机端一步生成音乐：自动按 50 积分报价、冻结积分并入队，
// 返回 generation_id 后，客户端轮询 GET /api/v1/generations/{id} 获取进度与作品。
// 请求体：{"prompt":"...","lyrics":"...","genre":"pop","mood":"happy","duration_seconds":60,"title":"...","visibility":"public","auto_publish":true}
func (s *Server) mobileMusic(w http.ResponseWriter, r *http.Request) {
	me, okUser := currentUser(w, r)
	if !okUser {
		return
	}

	var req struct {
		Prompt          string   `json:"prompt"`
		Lyrics          string   `json:"lyrics"`
		Genre           string   `json:"genre"`
		Styles          []string `json:"styles"`
		Mood            string   `json:"mood"`
		DurationSeconds int      `json:"duration_seconds"`
		Vocal           string   `json:"vocal"`
		Quality         string   `json:"quality"`
		Title           string   `json:"title"`
		Visibility      string   `json:"visibility"`
		AutoPublish     bool     `json:"auto_publish"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Prompt) == "" && strings.TrimSpace(req.Lyrics) == "" {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", "prompt or lyrics is required")
		return
	}

	styles := req.Styles
	if len(styles) == 0 && strings.TrimSpace(req.Genre) != "" {
		styles = []string{strings.TrimSpace(req.Genre)}
	}
	if len(styles) == 0 {
		styles = []string{"pop"}
	}
	if len(styles) > 3 {
		styles = styles[:3]
	}
	duration := req.DurationSeconds
	if duration == 0 {
		duration = 60
	}

	quote, err := s.core.CreateQuote(r.Context(), me.TenantID, me.UserID, core.QuoteRequest{
		Styles: styles, Duration: duration, Quality: req.Quality,
	})
	if err != nil {
		fail(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}

	gen, err := s.core.CreateGeneration(r.Context(), me.TenantID, me.UserID, core.GenerateRequest{
		QuoteID: quote.ID, Prompt: req.Prompt, Lyrics: req.Lyrics, Styles: styles,
		Mood: req.Mood, Duration: duration, Vocal: req.Vocal, Quality: req.Quality,
		Title: req.Title, Visibility: req.Visibility, AutoPublish: req.AutoPublish,
	})
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

	ok(w, map[string]any{
		"generation_id": gen.ID,
		"status":        gen.Status,
		"credits":       quote.Total,
		"poll_url":      "/api/v1/generations/" + gen.ID,
	})
}

func defaultArg(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
