package musicmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client 调用本地音乐大模型 HTTP 服务（bigMusicModel/web/server.py）。
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.BaseURL != ""
}

type GenerateRequest struct {
	Title    string `json:"title"`
	Lyrics   string `json:"lyrics"`
	Genre    string `json:"genre"`
	Mood     string `json:"mood"`
	BPM      int    `json:"bpm"`
	Duration int    `json:"duration"`
}

type Progress struct {
	Stage   string
	Percent int
	Message string
}

type submitResponse struct {
	OK     bool   `json:"ok"`
	TaskID string `json:"task_id"`
	Error  string `json:"error"`
}

type progressResponse struct {
	OK       bool   `json:"ok"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Stage    string `json:"stage"`
	Msg      string `json:"msg"`
	Error    string `json:"error"`
	Result   struct {
		WAVURL      string  `json:"wav_url"`
		DurationSec float64 `json:"duration_sec"`
		ElapsedSec  float64 `json:"elapsed_sec"`
	} `json:"result"`
}

// Result 是一次完整生成的结果：成品音频字节与实际时长（秒）。
type Result struct {
	Audio       []byte
	DurationSec float64
}

// Generate 提交生成任务、轮询进度并下载成品音频（WAV 字节）。
func (c *Client) Generate(ctx context.Context, req GenerateRequest, onProgress func(Progress)) (*Result, error) {
	if !c.Enabled() {
		return nil, errors.New("music model is not configured")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("submit generation: %w", err)
	}
	defer resp.Body.Close()
	var submit submitResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&submit); err != nil {
		return nil, fmt.Errorf("decode submit response: %w", err)
	}
	if resp.StatusCode != http.StatusAccepted || !submit.OK || submit.TaskID == "" {
		if submit.Error == "" {
			submit.Error = resp.Status
		}
		return nil, fmt.Errorf("submit generation: %s", submit.Error)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			p, err := c.poll(ctx, submit.TaskID)
			if err != nil {
				failures++
				if failures >= 5 {
					return nil, fmt.Errorf("poll generation: %w", err)
				}
				continue
			}
			failures = 0
			if onProgress != nil {
				onProgress(Progress{Stage: p.Stage, Percent: p.Progress, Message: p.Msg})
			}
			switch p.Status {
			case "done":
				if p.Result.WAVURL == "" {
					return nil, errors.New("generation finished without audio url")
				}
				audio, err := c.download(ctx, p.Result.WAVURL)
				if err != nil {
					return nil, err
				}
				return &Result{Audio: audio, DurationSec: p.Result.DurationSec}, nil
			case "failed":
				msg := p.Error
				if msg == "" {
					msg = "music model generation failed"
				}
				return nil, errors.New(msg)
			}
		}
	}
}

func (c *Client) poll(ctx context.Context, taskID string) (*progressResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/progress/"+taskID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var p progressResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&p); err != nil {
		return nil, err
	}
	if !p.OK {
		if p.Error == "" {
			p.Error = resp.Status
		}
		return nil, errors.New(p.Error)
	}
	return &p, nil
}

func (c *Client) download(ctx context.Context, path string) ([]byte, error) {
	url := path
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		url = c.BaseURL + "/" + strings.TrimLeft(path, "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download audio: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download audio: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
	if err != nil {
		return nil, err
	}
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errors.New("music model returned an invalid wav file")
	}
	return data, nil
}
