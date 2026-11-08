package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/model"
)

func (s *Store) ListNotifications(ctx context.Context, userID, filter string, limit int) ([]model.Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	q := `
		SELECT n.id, n.type, n.actor_id, COALESCE(u.display_name,''), COALESCE(u.avatar,''),
		       n.title, n.body, n.target_type, n.target_id, n.read_at, n.created_at
		FROM notifications n LEFT JOIN users u ON u.id=n.actor_id
		WHERE n.user_id=$1`
	args := []any{userID}
	if filter != "" && filter != "all" {
		q += ` AND n.type=$2`
		args = append(args, filter)
	}
	q += ` ORDER BY n.created_at DESC LIMIT $` + itoa(len(args)+1)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Notification{}
	for rows.Next() {
		var n model.Notification
		var read sql.NullTime
		if err := rows.Scan(&n.ID, &n.Type, &n.ActorID, &n.ActorName, &n.ActorAvatar,
			&n.Title, &n.Body, &n.TargetType, &n.TargetID, &read, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.ReadAt = timePtr(read)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) MarkAllRead(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE notifications SET read_at=now() WHERE user_id=$1 AND read_at IS NULL`, userID)
	return err
}

func (s *Store) MarkNotificationRead(ctx context.Context, id, userID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE notifications SET read_at=now() WHERE id=$1 AND user_id=$2`, id, userID)
	return err
}

func (s *Store) UnreadCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

func (s *Store) Conversations(ctx context.Context, userID string) ([]model.Conversation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, peer.id, peer.display_name, peer.avatar,
		       COALESCE((SELECT text FROM messages m WHERE m.conversation_id=c.id
		                 ORDER BY m.created_at DESC LIMIT 1), ''),
		       cm.unread_count, cm.pinned, c.last_message_at
		FROM conversations c
		JOIN conversation_members cm ON cm.conversation_id=c.id AND cm.user_id=$1
		JOIN conversation_members om ON om.conversation_id=c.id AND om.user_id<>$1
		JOIN users peer ON peer.id=om.user_id
		ORDER BY cm.pinned DESC, c.last_message_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Conversation{}
	for rows.Next() {
		var c model.Conversation
		if err := rows.Scan(&c.ID, &c.PeerID, &c.PeerName, &c.PeerAvatar, &c.LastMessage,
			&c.Unread, &c.Pinned, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) EnsureDirectConversation(ctx context.Context, a, b string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id FROM conversations c
		JOIN conversation_members m1 ON m1.conversation_id=c.id AND m1.user_id=$1
		JOIN conversation_members m2 ON m2.conversation_id=c.id AND m2.user_id=$2
		LIMIT 1`, a, b).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id = NewID("cv_")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO conversations(id, last_message_at) VALUES($1,now())`, id); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO conversation_members(conversation_id, user_id) VALUES($1,$2),($1,$3)`,
		id, a, b); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) IsConversationMember(ctx context.Context, convID, userID string) bool {
	var ok bool
	_ = s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)`,
		convID, userID).Scan(&ok)
	return ok
}

func (s *Store) Messages(ctx context.Context, convID string, limit int) ([]model.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, sender_id, type, text, work_id, created_at
		FROM messages WHERE conversation_id=$1
		ORDER BY created_at DESC LIMIT $2`, convID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.Message{}
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.ID, &m.SenderID, &m.Type, &m.Text, &m.WorkID, &m.CreatedAt); err != nil {
			return nil, err
		}
		if m.WorkID != "" {
			if w, err := s.GetWorkAny(ctx, m.WorkID); err == nil {
				m.Work = w
			}
		}
		items = append(items, m)
	}
	// Return chronological order for the conversation UI.
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, rows.Err()
}

func (s *Store) SendMessage(ctx context.Context, convID, senderID, typ, text, workID string) (*model.Message, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var member bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)`,
		convID, senderID).Scan(&member); err != nil {
		return nil, err
	}
	if !member {
		return nil, errors.New("not a conversation member")
	}
	id := NewID("m_")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages(id, conversation_id, sender_id, type, text, work_id)
		VALUES($1,$2,$3,$4,$5,$6)`, id, convID, senderID, typ, text, workID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations SET last_message_at=now() WHERE id=$1`, convID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE conversation_members SET unread_count=unread_count+1
		WHERE conversation_id=$1 AND user_id<>$2`, convID, senderID); err != nil {
		return nil, err
	}
	var created time.Time
	if err := tx.QueryRowContext(ctx, `SELECT created_at FROM messages WHERE id=$1`, id).Scan(&created); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	m := &model.Message{ID: id, SenderID: senderID, Type: typ, Text: text, WorkID: workID, CreatedAt: created}
	if workID != "" {
		if w, err := s.GetWorkAny(ctx, workID); err == nil {
			m.Work = w
		}
	}
	return m, nil
}

func (s *Store) MarkConversationRead(ctx context.Context, convID, userID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE conversation_members SET unread_count=0 WHERE conversation_id=$1 AND user_id=$2`,
		convID, userID)
	return err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
