// Package music 调用远程音乐大模型（MuseFlow）：提交生成任务、轮询进度，
// 拿到音频地址后把音频下载到本地，再由调用方上传对象存储。
package music

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/config"
)

const (
	maxAudioBytes  = 200 << 20
	pollInterval   = 1500 * time.Millisecond
	generatePath   = "/api/generate"
	lyricsPath     = "/api/lyrics"
	progressPrefix = "/api/progress/"
)

// Client 是音乐大模型的 HTTP 客户端，未配置地址时 Enabled() 返回 false。
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
	enabled bool
}

func New(cfg config.Config) *Client {
	c := &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.MusicModelURL), "/"),
		apiKey:  strings.TrimSpace(cfg.MusicModelAPIKey),
	}
	timeout := cfg.MusicModelTimeout
	if timeout <= 0 {
		timeout = 150 * time.Second
	}
	c.http = &http.Client{Timeout: timeout}
	c.enabled = c.baseURL != ""
	return c
}

func (c *Client) Enabled() bool { return c != nil && c.enabled }

// Request 是提交给模型的生成参数。
type Request struct {
	GenerationID    string
	Prompt          string
	Lyrics          string
	Styles          []string
	Mood            string
	DurationSeconds int
	Vocal           string
	Quality         string
	Title           string
}

// Result 是模型生成的音频信息。
type Result struct {
	AudioURL string
	Duration int
}

type generatePayload struct {
	Title    string `json:"title"`
	Lyrics   string `json:"lyrics"`
	Genre    string `json:"genre"`
	Mood     string `json:"mood"`
	BPM      int    `json:"bpm"`
	Duration int    `json:"duration"`
	Vocal    string `json:"vocal"`
}

// LyricsRequest 是 AI 写词的入参。
type LyricsRequest struct {
	Theme string
	Genre string
	Mood  string
	BPM   int
}

type lyricsPayload struct {
	Theme string `json:"theme"`
	Genre string `json:"genre"`
	Mood  string `json:"mood"`
	BPM   int    `json:"bpm"`
}

type lyricsResponse struct {
	OK     bool   `json:"ok"`
	Lyrics string `json:"lyrics"`
	Error  string `json:"error"`
}

type submitResponse struct {
	OK     bool   `json:"ok"`
	TaskID string `json:"task_id"`
	Error  string `json:"error"`
}

type progressResult struct {
	WavURL      string  `json:"wav_url"`
	AudioURL    string  `json:"audio_url"`
	DurationSec float64 `json:"duration_sec"`
}

type progressResponse struct {
	Status   string          `json:"status"`
	Progress int             `json:"progress"`
	Stage    string          `json:"stage"`
	Message  string          `json:"msg"`
	Error    string          `json:"error"`
	Result   *progressResult `json:"result"`
}

// GenerateLyrics 调用模型写词接口，同步返回歌词文本。
func (c *Client) GenerateLyrics(ctx context.Context, req LyricsRequest) (string, error) {
	if !c.Enabled() {
		return "", errors.New("music model is not configured")
	}
	theme := strings.TrimSpace(req.Theme)
	if theme == "" {
		return "", errors.New("theme is required")
	}
	payload, err := json.Marshal(lyricsPayload{
		Theme: theme,
		Genre: genreValue([]string{req.Genre}),
		Mood:  moodValue(req.Mood),
		BPM:   bpmValue(req.BPM),
	})
	if err != nil {
		return "", err
	}

	resp, err := c.do(ctx, http.MethodPost, c.baseURL+lyricsPath, payload)
	if err != nil {
		return "", fmt.Errorf("call music lyrics: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var parsed lyricsResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode lyrics response: %w (body=%s)", err, strings.TrimSpace(string(raw)))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 || !parsed.OK || strings.TrimSpace(parsed.Lyrics) == "" {
		return "", fmt.Errorf("music model lyrics failed: %s %s", resp.Status, strings.TrimSpace(parsed.Error))
	}
	return strings.TrimSpace(parsed.Lyrics), nil
}

// Generate 提交生成任务并轮询到完成，返回音频地址（已补全为绝对地址）。
func (c *Client) Generate(ctx context.Context, req Request) (*Result, error) {
	if !c.Enabled() {
		return nil, errors.New("music model is not configured")
	}
	taskID, err := c.submit(ctx, req)
	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("music model task %s: %w", taskID, ctx.Err())
		case <-ticker.C:
			done, result, err := c.poll(ctx, taskID)
			if err != nil {
				return nil, err
			}
			if !done {
				continue
			}
			if result.AudioURL == "" {
				return nil, fmt.Errorf("music model task %s finished without audio url", taskID)
			}
			return result, nil
		}
	}
}

func (c *Client) submit(ctx context.Context, req Request) (string, error) {
	payload := generatePayload{
		Title:    firstRunes(defaultText(req.Title, req.Prompt), 40),
		Lyrics:   req.Lyrics,
		Genre:    genreValue(req.Styles),
		Mood:     moodValue(req.Mood),
		BPM:      90,
		Duration: durationValue(req.DurationSeconds),
		Vocal:    vocalValue(req.Vocal),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := c.do(ctx, http.MethodPost, c.baseURL+generatePath, body)
	if err != nil {
		return "", fmt.Errorf("submit music task: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var parsed submitResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode submit response: %w (body=%s)", err, strings.TrimSpace(string(raw)))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 || !parsed.OK || parsed.TaskID == "" {
		return "", fmt.Errorf("music model submit failed: %s %s", resp.Status, strings.TrimSpace(parsed.Error))
	}
	return parsed.TaskID, nil
}

// poll 查询一次任务进度；done=true 时表示任务已结束。
func (c *Client) poll(ctx context.Context, taskID string) (bool, *Result, error) {
	resp, err := c.do(ctx, http.MethodGet, c.baseURL+progressPrefix+url.PathEscape(taskID), nil)
	if err != nil {
		return false, nil, fmt.Errorf("poll music task: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil, fmt.Errorf("music task %s not found", taskID)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false, nil, fmt.Errorf("poll music task: %s %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var parsed progressResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return false, nil, fmt.Errorf("decode progress response: %w", err)
	}
	switch parsed.Status {
	case "failed":
		msg := parsed.Error
		if msg == "" {
			msg = parsed.Message
		}
		return false, nil, fmt.Errorf("music task failed: %s", msg)
	case "done":
		result := &Result{}
		if parsed.Result != nil {
			result.AudioURL = c.resolveURL(firstNonEmpty(parsed.Result.WavURL, parsed.Result.AudioURL))
			result.Duration = int(parsed.Result.DurationSec)
		}
		if result.AudioURL == "" {
			result.AudioURL = c.resolveURL(findAudioURL(raw))
		}
		return true, result, nil
	default:
		return false, nil, nil
	}
}

func (c *Client) do(ctx context.Context, method, target string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return c.http.Do(req)
}

// resolveURL 把模型返回的相对地址补全为绝对地址。
func (c *Client) resolveURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return base.ResolveReference(ref).String()
}

// Download 把模型返回的音频下载到本地文件，扩展名取自音频 URL。
func (c *Client) Download(ctx context.Context, audioURL, dir, baseName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("download model audio: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("download model audio: %s %s", resp.Status, strings.TrimSpace(string(detail)))
	}

	ext := AudioExt(audioURL, resp.Header.Get("Content-Type"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, baseName+ext)
	file, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := io.Copy(file, io.LimitReader(resp.Body, maxAudioBytes)); err != nil {
		return "", err
	}
	return dest, nil
}

// AudioExt 根据 URL 后缀或 Content-Type 推断音频扩展名。
func AudioExt(audioURL, contentType string) string {
	if u, err := url.Parse(audioURL); err == nil {
		switch strings.ToLower(filepath.Ext(u.Path)) {
		case ".mp3", ".wav", ".flac", ".m4a", ".ogg", ".aac":
			return strings.ToLower(filepath.Ext(u.Path))
		}
	}
	contentType = strings.ToLower(contentType)
	switch {
	case strings.Contains(contentType, "mpeg"), strings.Contains(contentType, "mp3"):
		return ".mp3"
	case strings.Contains(contentType, "flac"):
		return ".flac"
	case strings.Contains(contentType, "mp4"), strings.Contains(contentType, "m4a"):
		return ".m4a"
	case strings.Contains(contentType, "ogg"):
		return ".ogg"
	case strings.Contains(contentType, "aac"):
		return ".aac"
	default:
		return ".wav"
	}
}

var allowedGenres = map[string]bool{
	"pop": true, "folk": true, "campus_folk": true, "guofeng": true, "r_and_b": true, "country": true,
	"rock": true, "punk": true, "metal": true, "blues": true, "funk": true, "reggae": true,
	"jazz": true, "electronic": true, "hiphop": true, "rap": true, "classical": true, "instrumental": true,
}

var allowedMoods = map[string]bool{
	"peaceful": true, "healing": true, "sad": true, "melancholy": true, "happy": true, "romantic": true,
	"energetic": true, "tense": true, "mysterious": true, "suppressed": true, "awakening": true, "triumphant": true,
}

// genreValue 取首个曲风，模型只接受约定枚举，非法值回退 pop。
func genreValue(styles []string) string {
	if len(styles) > 0 {
		if v := strings.ToLower(strings.TrimSpace(styles[0])); allowedGenres[v] {
			return v
		}
	}
	return "pop"
}

// moodValue 只接受约定枚举，非法值回退 peaceful。
func moodValue(mood string) string {
	if v := strings.ToLower(strings.TrimSpace(mood)); allowedMoods[v] {
		return v
	}
	return "peaceful"
}

// vocalValue 把内部音色描述映射为模型的 auto/female/male。
func vocalValue(vocal string) string {
	switch strings.TrimSpace(vocal) {
	case "female", "女声":
		return "female"
	case "male", "男声":
		return "male"
	default:
		return "auto"
	}
}

// bpmValue 约束速度到模型支持的 60-150 区间，缺省 90。
func bpmValue(bpm int) int {
	switch {
	case bpm == 0:
		return 90
	case bpm < 60:
		return 60
	case bpm > 150:
		return 150
	default:
		return bpm
	}
}

// durationValue 收敛到模型支持的时长档位。
func durationValue(seconds int) int {
	switch {
	case seconds <= 15:
		return 15
	case seconds <= 30:
		return 30
	case seconds <= 60:
		return 60
	case seconds <= 180:
		return 180
	default:
		return 300
	}
}

func defaultText(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}

// findAudioURL 在响应中递归查找音频地址，兼容字段名差异。
func findAudioURL(v any) string {
	switch value := v.(type) {
	case map[string]any:
		for _, key := range []string{"audio_url", "audioUrl", "audioURL", "wav_url", "result_url", "file_url", "url", "audio"} {
			if raw, exist := value[key]; exist {
				if s, isStr := raw.(string); isStr && strings.TrimSpace(s) != "" {
					return s
				}
			}
		}
		for _, key := range []string{"data", "result", "output", "response"} {
			if child, exist := value[key]; exist {
				if found := findAudioURL(child); found != "" {
					return found
				}
			}
		}
	case []any:
		for _, item := range value {
			if found := findAudioURL(item); found != "" {
				return found
			}
		}
	}
	return ""
}
