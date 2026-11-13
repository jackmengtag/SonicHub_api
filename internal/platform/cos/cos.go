// Package cos 通过 COS XML API 把生成的音频上传到腾讯云对象存储。
// 只实现 PUT Object，使用 HMAC-SHA1 签名，避免引入额外的 SDK 依赖。
package cos

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/config"
)

const signExpire = time.Hour

type Client struct {
	secretID   string
	secretKey  string
	bucket     string
	region     string
	publicBase string
	acl        string
	enabled    bool
	http       *http.Client
}

// New 根据配置创建 COS 客户端；AK/SK/桶/地域缺一不可，缺省时 Enabled() 返回 false。
func New(cfg config.Config) (*Client, error) {
	c := &Client{
		secretID:   strings.TrimSpace(cfg.COSSecretID),
		secretKey:  strings.TrimSpace(cfg.COSSecretKey),
		bucket:     strings.TrimSpace(cfg.COSBucket),
		region:     strings.TrimSpace(cfg.COSRegion),
		publicBase: strings.TrimRight(strings.TrimSpace(cfg.COSPublicBase), "/"),
		acl:        strings.TrimSpace(cfg.COSACL),
		http:       &http.Client{Timeout: 120 * time.Second},
	}
	if c.acl == "" {
		c.acl = "public-read"
	}
	if c.acl == "default" {
		c.acl = ""
	}
	if c.secretID == "" || c.secretKey == "" || c.bucket == "" || c.region == "" {
		return c, nil
	}
	c.enabled = true
	return c, nil
}

func (c *Client) Enabled() bool { return c != nil && c.enabled }

// UploadFile 上传本地文件，返回可访问的 URL。
func (c *Client) UploadFile(ctx context.Context, key, localPath, contentType string) (string, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	return c.upload(ctx, key, file, info.Size(), contentType)
}

// Upload 上传内存/流数据（远程模型直接返回音频字节时使用），返回可访问的 URL。
func (c *Client) Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) (string, error) {
	return c.upload(ctx, key, body, size, contentType)
}

func (c *Client) upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) (string, error) {
	if !c.Enabled() {
		return "", errors.New("cos is not configured")
	}
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return "", errors.New("cos object key is empty")
	}
	if contentType == "" {
		contentType = "audio/wav"
	}

	host := c.host()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "https://"+host+"/"+escapeKey(key), body)
	if err != nil {
		return "", err
	}
	if size >= 0 {
		req.ContentLength = size
	}
	req.Header.Set("Content-Type", contentType)
	if c.acl != "" {
		req.Header.Set("x-cos-acl", c.acl)
	}
	req.Header.Set("Authorization", c.authorization(http.MethodPut, "/"+key, host, contentType))
	req.Header.Set("Host", host)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("cos upload failed: %s %s", resp.Status, strings.TrimSpace(string(detail)))
	}
	return c.publicURL(key), nil
}

func (c *Client) host() string {
	return fmt.Sprintf("%s.cos.%s.myqcloud.com", c.bucket, c.region)
}

func (c *Client) publicURL(key string) string {
	if c.publicBase != "" {
		return c.publicBase + "/" + escapeKey(key)
	}
	return "https://" + c.host() + "/" + escapeKey(key)
}

// authorization 生成 COS 请求签名，签名头为 content-type、host，配置了 ACL 时追加 x-cos-acl。
func (c *Client) authorization(method, uriPath, host, contentType string) string {
	now := time.Now().Unix()
	signTime := strconv.FormatInt(now, 10) + ";" + strconv.FormatInt(now+int64(signExpire/time.Second), 10)
	signKey := hmacSHA1(c.secretKey, signTime)

	headerList := "content-type;host"
	headers := "content-type=" + urlEncode(contentType) + "&host=" + urlEncode(host)
	if c.acl != "" {
		headerList = "content-type;host;x-cos-acl"
		headers += "&x-cos-acl=" + urlEncode(c.acl)
	}
	httpString := strings.ToLower(method) + "\n" + uriPath + "\n\n" + headers + "\n"
	stringToSign := "sha1\n" + signTime + "\n" + sha1Hex(httpString) + "\n"
	signature := hmacSHA1(signKey, stringToSign)

	return "q-sign-algorithm=sha1" +
		"&q-ak=" + urlEncode(c.secretID) +
		"&q-sign-time=" + signTime +
		"&q-key-time=" + signTime +
		"&q-header-list=" + headerList +
		"&q-url-param-list=" +
		"&q-signature=" + signature
}

func hmacSHA1(key, data string) string {
	h := hmac.New(sha1.New, []byte(key))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func sha1Hex(data string) string {
	sum := sha1.Sum([]byte(data))
	return hex.EncodeToString(sum[:])
}

func urlEncode(v string) string {
	return strings.ReplaceAll(url.QueryEscape(v), "+", "%20")
}

// escapeKey 逐段转义对象键，保留路径分隔符。
func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
