package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr          string
	DatabaseURL   string
	JWTSecret     string
	DemoCode      string
	SeedDemo      bool
	MediaDir      string
	MigrationsDir string
	AdminPhone    string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	// AllowDemoCode 允许继续使用 DEMO_AUTH_CODE 登录（仅本地调试开启，生产必须关闭）。
	AllowDemoCode bool
	// SMSProvider 短信发送通道：dypns（号码认证服务）或 dysms（短信服务自建验证码）。
	SMSProvider string

	// 阿里云号码认证服务（Dypnsapi）短信验证码配置；未配置时回退到演示验证码。
	AliyunAccessKeyID      string
	AliyunAccessKeySecret  string
	AliyunSMSSignName      string
	AliyunSMSTemplateCode  string
	AliyunSMSTemplateParam string
	AliyunSMSSchemeName    string
	AliyunSMSSceneCode     string
	AliyunSMSEndpoint      string
}

// SMSEnabled 表示是否已配置完整的阿里云短信认证参数。
func (c Config) SMSEnabled() bool {
	return strings.TrimSpace(c.AliyunAccessKeyID) != "" &&
		strings.TrimSpace(c.AliyunAccessKeySecret) != "" &&
		strings.TrimSpace(c.AliyunSMSSignName) != "" &&
		strings.TrimSpace(c.AliyunSMSTemplateCode) != ""
}

// SMSSDKEnabled 表示是否已配置 Android 短信验证码 SDK 所需的参数（方案号 + 模板）。
func (c Config) SMSSDKEnabled() bool {
	return strings.TrimSpace(c.AliyunAccessKeyID) != "" &&
		strings.TrimSpace(c.AliyunAccessKeySecret) != "" &&
		strings.TrimSpace(c.AliyunSMSSceneCode) != "" &&
		strings.TrimSpace(c.AliyunSMSTemplateCode) != ""
}

func Load() Config {
	loadDotEnv(".env")
	return Config{
		Addr:          env("HTTP_ADDR", ":8080"),
		DatabaseURL:   env("DATABASE_URL", "postgres://sonichub:sonichub@localhost:5432/sonichub?sslmode=disable"),
		JWTSecret:     env("JWT_SECRET", "sonichub-dev-secret-change-me"),
		DemoCode:      env("DEMO_AUTH_CODE", "1234"),
		SeedDemo:      envBool("SEED_DEMO", true),
		MediaDir:      env("MEDIA_DIR", "./data/media"),
		MigrationsDir: env("MIGRATIONS_DIR", "./migrations"),
		AdminPhone:    env("ADMIN_PHONE", "13800138000"),
		AccessTTL:     time.Duration(envInt("ACCESS_TOKEN_MINUTES", 1440)) * time.Minute,
		RefreshTTL:    time.Duration(envInt("REFRESH_TOKEN_DAYS", 30)) * 24 * time.Hour,
		AllowDemoCode: envBool("ALLOW_DEMO_CODE", false),
		SMSProvider:   strings.ToLower(env("SMS_PROVIDER", "dypns")),

		AliyunAccessKeyID:      env("ALIYUN_ACCESS_KEY_ID", ""),
		AliyunAccessKeySecret:  env("ALIYUN_ACCESS_KEY_SECRET", ""),
		AliyunSMSSignName:      env("ALIYUN_SMS_SIGN_NAME", ""),
		AliyunSMSTemplateCode:  env("ALIYUN_SMS_TEMPLATE_CODE", ""),
		AliyunSMSTemplateParam: env("ALIYUN_SMS_TEMPLATE_PARAM", `{"code":"##code##","min":"5"}`),
		AliyunSMSSchemeName:    env("ALIYUN_SMS_SCHEME_NAME", ""),
		AliyunSMSSceneCode:     env("ALIYUN_SMS_SCENE_CODE", ""),
		AliyunSMSEndpoint:      env("ALIYUN_SMS_ENDPOINT", "dypnsapi.aliyuncs.com"),
	}
}

// loadDotEnv 读取工作目录下的 .env（KEY=VALUE），已存在的环境变量优先。
func loadDotEnv(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
