package model

import "time"

type User struct {
	ID          string    `json:"id"`
	Phone       string    `json:"phone,omitempty"`
	DisplayName string    `json:"display_name"`
	Avatar      string    `json:"avatar"`
	Bio         string    `json:"bio"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	Genres      []string  `json:"genres"`
	CreatedAt   time.Time `json:"created_at"`
}

type Musician struct {
	UserID     string     `json:"user_id"`
	StageName  string     `json:"stage_name"`
	Status     string     `json:"status"`
	Bio        string     `json:"bio"`
	Genres     []string   `json:"genres"`
	AppliedAt  time.Time  `json:"applied_at"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
}

type UserCard struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	Bio       string `json:"bio"`
	Verified  bool   `json:"verified"`
	StageName string `json:"stage_name,omitempty"`
	Following bool   `json:"following"`
	Friend    bool   `json:"friend"`
	Follower  bool   `json:"follower"`
	Blocked   bool   `json:"blocked"`
}

type Work struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Cover         string     `json:"cover"`
	Description   string     `json:"description"`
	Visibility    string     `json:"visibility"`
	Status        string     `json:"status"`
	RejectReason  string     `json:"reject_reason,omitempty"`
	Genre         string     `json:"genre"`
	Tags          []string   `json:"tags"`
	AudioURL      string     `json:"audio_url"`
	Duration      int        `json:"duration_seconds"`
	PlayCount     int        `json:"play_count"`
	LikeCount     int        `json:"like_count"`
	CommentCount  int        `json:"comment_count"`
	ShareCount    int        `json:"share_count"`
	IsAI          bool       `json:"is_ai"`
	OwnerID       string     `json:"owner_id"`
	OwnerName     string     `json:"owner_name"`
	OwnerAvatar   string     `json:"owner_avatar"`
	OwnerVerified bool       `json:"owner_verified"`
	Liked         bool       `json:"liked"`
	Collected     bool       `json:"collected"`
	PublishedAt   *time.Time `json:"published_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	Reason        string     `json:"reason,omitempty"`
}

type Comment struct {
	ID        string    `json:"id"`
	WorkID    string    `json:"work_id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Avatar    string    `json:"avatar"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Notification struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	ActorID     string     `json:"actor_id,omitempty"`
	ActorName   string     `json:"actor_name,omitempty"`
	ActorAvatar string     `json:"actor_avatar,omitempty"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	TargetType  string     `json:"target_type"`
	TargetID    string     `json:"target_id"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Conversation struct {
	ID          string    `json:"id"`
	PeerID      string    `json:"peer_id"`
	PeerName    string    `json:"peer_name"`
	PeerAvatar  string    `json:"peer_avatar"`
	LastMessage string    `json:"last_message"`
	Unread      int       `json:"unread"`
	Pinned      bool      `json:"pinned"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Message struct {
	ID        string    `json:"id"`
	SenderID  string    `json:"sender_id"`
	Type      string    `json:"type"`
	Text      string    `json:"text"`
	WorkID    string    `json:"work_id,omitempty"`
	Work      *Work     `json:"work,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type LedgerEntry struct {
	ID           string    `json:"id"`
	EntryType    string    `json:"entry_type"`
	Amount       int64     `json:"amount"`
	BalanceAfter int64     `json:"balance_after"`
	RefType      string    `json:"ref_type"`
	RefID        string    `json:"ref_id"`
	CreatedAt    time.Time `json:"created_at"`
}

type CreditAccount struct {
	Available int64 `json:"available"`
	Reserved  int64 `json:"reserved"`
}

type CheckinResult struct {
	Date    string `json:"date"`
	Already bool   `json:"already_checked_in"`
	Credits int64  `json:"credits"`
	Balance int64  `json:"balance"`
}

type CheckinStatus struct {
	Date          string `json:"date"`
	CheckedIn     bool   `json:"checked_in"`
	CreditsPerDay int64  `json:"credits_per_day"`
	Balance       int64  `json:"balance"`
}

type Quote struct {
	ID          string    `json:"quote_id"`
	TenantID    string    `json:"-"`
	Base        int       `json:"base"`
	QualityFee  int       `json:"quality_fee"`
	StemsFee    int       `json:"stems_fee"`
	DurationFee int       `json:"duration_fee"`
	Total       int       `json:"total"`
	Status      string    `json:"status"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Generation struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"-"`
	UserID       string     `json:"-"`
	Status       string     `json:"status"`
	Stage        string     `json:"stage"`
	Progress     int        `json:"progress"`
	Prompt       string     `json:"prompt"`
	Lyrics       string     `json:"lyrics"`
	Styles       []string   `json:"styles"`
	Mood         string     `json:"mood"`
	Duration     int        `json:"duration_seconds"`
	Vocal        string     `json:"vocal"`
	Quality      string     `json:"quality"`
	Stems        bool       `json:"stems"`
	Title        string     `json:"title"`
	Visibility   string     `json:"visibility"`
	AutoPublish  bool       `json:"auto_publish"`
	ModelVersion string     `json:"model_version"`
	Error        string     `json:"error,omitempty"`
	WorkID       string     `json:"work_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

type Package struct {
	ID         string `json:"id"`
	Credits    int    `json:"credits"`
	PriceCents int    `json:"price_cents"`
	Tag        string `json:"tag"`
}

type ModerationItem struct {
	ID         string    `json:"id"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Title      string    `json:"title"`
	Author     string    `json:"author"`
	Reason     string    `json:"reason"`
	Decision   string    `json:"decision"`
	Status     string    `json:"status"`
	Cover      string    `json:"cover,omitempty"`
	Genre      string    `json:"genre,omitempty"`
	Duration   int       `json:"duration_seconds,omitempty"`
	Visibility string    `json:"visibility,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Report struct {
	ID          string    `json:"id"`
	TargetType  string    `json:"target_type"`
	TargetID    string    `json:"target_id"`
	TargetTitle string    `json:"target_title"`
	Reporter    string    `json:"reporter"`
	Reason      string    `json:"reason"`
	Detail      string    `json:"detail"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type Appeal struct {
	ID        string    `json:"id"`
	WorkID    string    `json:"work_id"`
	WorkTitle string    `json:"work_title"`
	UserID    string    `json:"user_id"`
	Reason    string    `json:"reason"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type FeedPolicy struct {
	ID          string         `json:"id"`
	Version     string         `json:"version"`
	Weights     map[string]any `json:"weights"`
	Diversity   map[string]any `json:"diversity"`
	Status      string         `json:"status"`
	EffectiveAt time.Time      `json:"effective_at"`
}
