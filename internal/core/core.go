package core

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackmengtag/SonicHub_api/internal/model"
	"github.com/jackmengtag/SonicHub_api/internal/platform/storage"
	"github.com/jackmengtag/SonicHub_api/internal/store"
)

type Core struct {
	Store    *store.Store
	MediaDir string
}

func New(st *store.Store, mediaDir string) *Core {
	return &Core{Store: st, MediaDir: mediaDir}
}

type QuoteRequest struct {
	Styles   []string `json:"styles"`
	Duration int      `json:"duration_seconds"`
	Quality  string   `json:"quality"`
	Stems    bool     `json:"stems"`
}

// CreditsPerSong 是调用音乐大模型生成一首歌消耗的积分（5 元 = 50 积分，按 10 积分/元换算）。
const CreditsPerSong = 50

func (c *Core) CreateQuote(ctx context.Context, tenantID, userID string, req QuoteRequest) (*model.Quote, error) {
	if err := validateParams(req.Styles, req.Duration, req.Quality); err != nil {
		return nil, err
	}
	return c.Store.CreateQuote(ctx, tenantID, userID, CreditsPerSong, 0, 0, 0, req)
}

type GenerateRequest struct {
	QuoteID     string   `json:"quote_id"`
	Prompt      string   `json:"prompt"`
	Lyrics      string   `json:"lyrics"`
	Styles      []string `json:"styles"`
	Mood        string   `json:"mood"`
	Duration    int      `json:"duration_seconds"`
	Vocal       string   `json:"vocal"`
	Quality     string   `json:"quality"`
	Stems       bool     `json:"stems"`
	Title       string   `json:"title"`
	Visibility  string   `json:"visibility"`
	AutoPublish bool     `json:"auto_publish"`
}

// InsufficientCreditError 表示用户可用积分不足以支付一次生成。
type InsufficientCreditError struct {
	Required  int64
	Available int64
}

func (e *InsufficientCreditError) Error() string {
	return fmt.Sprintf("积分不足：生成一首歌需要 %d 积分，当前可用 %d 积分", e.Required, e.Available)
}

func (c *Core) CreateGeneration(ctx context.Context, tenantID, userID string, req GenerateRequest) (*model.Generation, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("prompt is required")
	}
	if utf8.RuneCountInString(req.Prompt) > 500 {
		return nil, errors.New("prompt is too long")
	}
	if err := validateParams(req.Styles, req.Duration, req.Quality); err != nil {
		return nil, err
	}
	if req.Visibility == "" {
		req.Visibility = "public"
	}
	if req.Visibility != "public" && req.Visibility != "friends" && req.Visibility != "private" {
		return nil, errors.New("invalid visibility")
	}

	quote, err := c.Store.GetQuote(ctx, req.QuoteID)
	if err != nil {
		return nil, err
	}
	if quote.TenantID != tenantID {
		return nil, errors.New("quote does not belong to this account")
	}
	if quote.Status != "open" {
		return nil, errors.New("quote is no longer valid")
	}
	if time.Now().After(quote.ExpiresAt) {
		return nil, errors.New("quote expired")
	}

	// 先按报价校验余额：积分不足时不创建任务，也不会调用音乐模型。
	account, err := c.Store.Account(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if account.Available < int64(quote.Total) {
		return nil, &InsufficientCreditError{Required: int64(quote.Total), Available: account.Available}
	}

	gen, err := c.Store.CreateGeneration(ctx, store.CreateGenerationInput{
		TenantID: tenantID, UserID: userID, QuoteID: quote.ID,
		Prompt: req.Prompt, Lyrics: req.Lyrics, Styles: req.Styles, Mood: req.Mood,
		Duration: req.Duration, Vocal: defaultString(req.Vocal, "女声"),
		Quality: defaultString(req.Quality, "标准"), Stems: req.Stems,
		Title: req.Title, Visibility: req.Visibility, AutoPublish: req.AutoPublish,
	})
	if err != nil {
		return nil, err
	}
	if err := c.Store.Reserve(ctx, gen.ID, quote.ID, tenantID, quote.Total); err != nil {
		_ = c.Store.FailGeneration(ctx, gen.ID, "credit reservation failed")
		if errors.Is(err, store.ErrInsufficientCredit) {
			available := account.Available
			if latest, aerr := c.Store.Account(ctx, tenantID); aerr == nil {
				available = latest.Available
			}
			return nil, &InsufficientCreditError{Required: int64(quote.Total), Available: available}
		}
		return nil, err
	}
	// 积分冻结成功后任务才进入队列，Worker 只会领取 queued 任务。
	if err := c.Store.QueueGeneration(ctx, gen.ID); err != nil {
		_ = c.Store.FailGeneration(ctx, gen.ID, "queue failed")
		_ = c.Store.Release(ctx, gen.ID, tenantID)
		return nil, err
	}
	gen.Status = "queued"
	gen.Stage = "queued"
	return gen, nil
}

func (c *Core) ProcessGeneration(ctx context.Context, gen *model.Generation) error {
	if err := c.Store.UpdateProgress(ctx, gen.ID, "composing", 32); err != nil {
		return err
	}
	time.Sleep(700 * time.Millisecond)

	if err := c.Store.UpdateProgress(ctx, gen.ID, "rendering", 58); err != nil {
		return err
	}
	time.Sleep(700 * time.Millisecond)

	if err := c.Store.UpdateProgress(ctx, gen.ID, "mixing", 80); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	outPath := filepath.Join(c.MediaDir, "generated", gen.ID+".wav")
	if err := storage.GenerateDemoWAV(outPath, gen.Duration, gen.ID); err != nil {
		return fmt.Errorf("generate audio: %w", err)
	}
	audioURL := "/media/generated/" + gen.ID + ".wav"

	title := strings.TrimSpace(gen.Title)
	if title == "" {
		title = firstRunes(gen.Prompt, 14)
	}
	decision, reason := Moderate(title, gen.Prompt+" "+gen.Lyrics)
	status := "draft"
	var publishedAt *time.Time
	if decision == "reject" {
		status = "rejected"
	} else if gen.AutoPublish {
		status = "published"
		now := time.Now()
		publishedAt = &now
	}

	cover := coverFor(gen.ID)
	genre := ""
	if len(gen.Styles) > 0 {
		genre = gen.Styles[0]
	}
	workID, err := c.Store.CreateWork(ctx, store.CreateWorkInput{
		TenantID: gen.TenantID, UserID: gen.UserID, Title: title, Cover: cover,
		Description: "", Visibility: gen.Visibility, Status: status,
		RejectReason: reasonIf(decision, reason), Genre: genre, Tags: gen.Styles,
		AudioURL: audioURL, Duration: gen.Duration, IsAI: true,
		GenerationID: gen.ID, PublishedAt: publishedAt,
	})
	if err != nil {
		return fmt.Errorf("create work: %w", err)
	}
	_ = c.Store.SaveGenerationWork(ctx, gen.ID, workID)
	if err := c.Store.CompleteGeneration(ctx, gen.ID, workID); err != nil {
		return err
	}
	gen.WorkID = workID
	gen.Status = "succeeded"
	gen.Progress = 100
	if err := c.Store.Capture(ctx, gen.ID, gen.TenantID); err != nil {
		slog.Error("capture credit failed", "generation", gen.ID, "error", err)
	}

	body := "作品已生成，可在「我的作品」中试听与发布"
	if decision == "reject" {
		body = "作品已生成，但未通过内容审核：" + reason
	}
	_ = c.Store.Notify(ctx, gen.UserID, "work", "", "创作完成：《"+title+"》", body, "work", workID, "gen:"+gen.ID)
	return nil
}

type PublishRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Visibility  string   `json:"visibility"`
	Tags        []string `json:"tags"`
}

func (c *Core) Publish(ctx context.Context, workID, userID string, req PublishRequest) (*model.Work, error) {
	work, err := c.Store.GetWorkAny(ctx, workID)
	if err != nil {
		return nil, err
	}
	if work.OwnerID != userID {
		return nil, errors.New("forbidden")
	}
	if req.Title == "" {
		req.Title = work.Title
	}
	if req.Visibility == "" {
		req.Visibility = work.Visibility
	}
	if req.Visibility != "public" && req.Visibility != "friends" && req.Visibility != "private" {
		return nil, errors.New("invalid visibility")
	}
	decision, reason := Moderate(req.Title, req.Description)
	status := "published"
	if decision == "reject" {
		status = "rejected"
	}
	tags := req.Tags
	if len(tags) == 0 {
		tags = work.Tags
	}
	if err := c.Store.PublishWork(ctx, workID, userID, req.Title, req.Description, req.Visibility, tags, status, reasonIf(decision, reason)); err != nil {
		return nil, err
	}
	title := "作品已发布"
	body := "《" + req.Title + "》已通过审核并发布"
	if status == "rejected" {
		title = "作品未通过审核"
		body = "《" + req.Title + "》未通过审核：" + reason
	}
	_ = c.Store.Notify(ctx, userID, "work", "", title, body, "work", workID, "publish:"+workID)
	return c.Store.GetWork(ctx, workID, userID)
}

func (c *Core) Cancel(ctx context.Context, genID, userID string) (*model.Generation, error) {
	gen, err := c.Store.CancelGeneration(ctx, genID, userID)
	if err != nil {
		return gen, err
	}
	if gen.Status == "cancelled" {
		if err := c.Store.Release(ctx, genID, gen.TenantID); err != nil {
			slog.Error("release credit failed", "generation", genID, "error", err)
		}
	}
	return gen, nil
}

var bannedWords = []string{"加微信", "赌博", "毒品", "色情", "暴力", "代写", "抄袭"}

func Moderate(title, description string) (decision string, reason string) {
	text := title + " " + description
	for _, w := range bannedWords {
		if strings.Contains(text, w) {
			return "reject", "内容包含违规词：" + w
		}
	}
	return "allow", ""
}

func reasonIf(decision, reason string) string {
	if decision == "reject" {
		return reason
	}
	return ""
}

func validateParams(styles []string, duration int, quality string) error {
	if len(styles) == 0 || len(styles) > 3 {
		return errors.New("styles must be between 1 and 3")
	}
	switch duration {
	case 15, 30, 60, 90, 120:
	default:
		return errors.New("invalid duration")
	}
	if quality != "" && quality != "标准" && quality != "高清" && quality != "standard" && quality != "high" {
		return errors.New("invalid quality")
	}
	return nil
}

func defaultString(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func firstRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}

func coverFor(seed string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	covers := []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7", "c8"}
	return covers[int(h.Sum32())%len(covers)]
}
