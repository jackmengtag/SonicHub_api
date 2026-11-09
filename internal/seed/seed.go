package seed

import (
	"context"
	"database/sql"
	"fmt"
)

// Seed inserts the demo dataset used by the prototype. It is idempotent and
// only runs when the users table is empty.
func Seed(ctx context.Context, db *sql.DB, adminPhone string) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	type user struct {
		id, phone, name, avatar, bio, role, genres string
	}
	users := []user{
		{"u_self", "13800138000", "沈知", "c6", "写歌给失眠的人听。", "admin", `["电子","Lo-fi"]`},
		{"u_linan", "13900000001", "林岸", "c1", "把城市噪音写进歌里。电子与 Lo-fi 制作人。", "user", `["电子","Lo-fi"]`},
		{"u_alan", "13900000002", "阿岚", "c3", "国风与影视配乐，写山水也写人间。", "user", `["国风","影视配乐"]`},
		{"u_yiming", "13900000003", "陈一鸣", "c2", "深夜 Lo-fi，便利店与末班车。", "user", `["Lo-fi","爵士"]`},
		{"u_xiaoye", "13900000004", "小野", "c4", "夏天、汽水和不重要的快乐。", "user", `["流行","电子"]`},
		{"u_zhouchi", "13900000005", "周迟", "c5", "R&B 新手，正在学习用 AI 写歌。", "user", `["R&B","情绪"]`},
		{"u_gubei", "13900000006", "顾北", "c6", "旧唱片收藏者，爵士与城市夜晚。", "user", `["爵士","Lo-fi"]`},
		{"u_bailu", "13900000007", "白露", "c7", "纯音乐，给电影和睡眠。", "user", `["国风","纯音乐"]`},
		{"u_kai", "13900000008", "KAI", "c8", "电子摇滚，现场型选手。", "user", `["电子","摇滚"]`},
	}
	if adminPhone != "" {
		users[0].phone = adminPhone
	}
	for _, u := range users {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO users(id, phone, display_name, avatar, bio, role, status, genres)
			VALUES($1,$2,$3,$4,$5,$6,'active',$7::jsonb)`,
			u.id, u.phone, u.name, u.avatar, u.bio, u.role, u.genres); err != nil {
			return fmt.Errorf("seed user %s: %w", u.id, err)
		}
		tenant := "t_" + u.id[2:]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tenants(id, type, name, owner_user_id) VALUES($1,'personal',$2,$3)`,
			tenant, u.name+"的空间", u.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO memberships(tenant_id, user_id, role) VALUES($1,$2,'owner')`, tenant, u.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO credit_accounts(tenant_id, available, reserved) VALUES($1,100,0)`, tenant); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
			VALUES($1,$2,'grant',100,100,$3,'grant',$4)`,
			"cl_grant_"+u.id, tenant, "grant:"+u.id, u.id); err != nil {
			return err
		}
	}

	musicians := []struct {
		user, stage, status, bio, genres, verified string
	}{
		{"u_self", "沈知", "pending", "写歌给失眠的人听。用 AI 记录城市夜晚的情绪。", `["电子","Lo-fi"]`, ""},
		{"u_linan", "林岸", "verified", "把城市噪音写进歌里。", `["电子","Lo-fi"]`, "2024-03-01"},
		{"u_alan", "阿岚", "verified", "国风与影视配乐。", `["国风","影视配乐"]`, "2024-05-12"},
		{"u_yiming", "陈一鸣", "verified", "深夜 Lo-fi。", `["Lo-fi","爵士"]`, "2024-06-20"},
		{"u_xiaoye", "小野", "verified", "夏天与汽水。", `["流行","电子"]`, "2024-07-02"},
		{"u_gubei", "顾北", "verified", "旧唱片收藏者。", `["爵士","Lo-fi"]`, "2024-02-14"},
		{"u_bailu", "白露", "verified", "纯音乐与电影。", `["国风","纯音乐"]`, "2024-01-08"},
		{"u_kai", "KAI", "verified", "电子摇滚。", `["电子","摇滚"]`, "2023-12-01"},
	}
	for _, m := range musicians {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO musicians(user_id, stage_name, status, bio, genres, applied_at, verified_at)
			VALUES($1,$2,$3,$4,$5::jsonb,now(),NULLIF($6,'')::timestamptz)`,
			m.user, m.stage, m.status, m.bio, m.genres, m.verified); err != nil {
			return err
		}
	}

	insertWork := func(id, tenant, owner, title, cover, desc, visibility, status, reason, genre, tags, audio string, dur, plays, likes, comments, shares int, publishedOffset string) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO works(id, tenant_id, user_id, title, cover, description, visibility, status,
				reject_reason, genre, tags, audio_url, duration_seconds, play_count, like_count,
				comment_count, share_count, is_ai, generation_id, published_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13,$14,$15,$16,$17,true,'',
				CASE WHEN $18='' THEN NULL ELSE now()-$18::interval END)`,
			id, tenant, owner, title, cover, desc, visibility, status, reason, genre, tags, audio,
			dur, plays, likes, comments, shares, publishedOffset)
		return err
	}

	works := []struct {
		id, tenant, owner, title, cover, desc, vis, status, reason, genre, tags, audio string
		dur, plays, likes, comments, shares                                            int
		published                                                                      string
	}{
		{"w_night", "t_self", "u_self", "夜航", "c2", "凌晨两点的城市，和一条没有尽头的路。", "public", "published", "", "电子", `["电子","Lo-fi"]`, "", 30, 18000, 642, 28, 41, "2 days"},
		{"w_hearing", "t_self", "u_self", "你听得到吗", "c4", "写给一个很久没有联系的人。", "public", "pending_review", "", "流行", `["流行","情绪"]`, "", 30, 0, 0, 0, 0, ""},
		{"w_blank", "t_self", "u_self", "空白房间", "c5", "一间没有窗户的房间。", "public", "rejected", "封面含未授权素材，请更换封面后重新提交", "R&B", `["R&B","情绪"]`, "", 30, 0, 0, 0, 0, ""},
		{"w_summer_demo", "t_self", "u_self", "夏末（草稿）", "c6", "还没写完的夏天。", "private", "draft", "", "流行", `["流行"]`, "", 30, 0, 0, 0, 0, ""},
		{"w_unnamed", "t_self", "u_self", "未命名作品 03", "c1", "朋友可见的试验作品。", "friends", "published", "", "电子", `["电子","Lo-fi"]`, "", 30, 126, 18, 2, 3, "2 hours"},
		{"w_alley", "t_self", "u_self", "旧巷", "c8", "雨后的旧巷，有猫和收音机。", "public", "published", "", "电子", `["电子","氛围"]`, "", 45, 3204, 208, 12, 9, "7 days"},
		{"w_mist", "t_linan", "u_linan", "雾中骑行", "c1", "晨雾中的城市骑行。", "public", "published", "", "电子", `["电子","Lo-fi"]`, "", 30, 124000, 3241, 128, 260, "2 hours"},
		{"w_store", "t_yiming", "u_yiming", "凌晨四点的便利店", "c2", "便利店的白灯和关东煮。", "public", "published", "", "Lo-fi", `["Lo-fi","爵士"]`, "", 60, 87000, 2108, 96, 188, "4 hours"},
		{"w_mountain", "t_alan", "u_alan", "山海之间", "c3", "山与海之间，有一段没有名字的路。", "public", "published", "", "国风", `["国风","影视配乐"]`, "", 45, 216000, 5860, 302, 510, "6 hours"},
		{"w_summer", "t_xiaoye", "u_xiaoye", "夏日漫游", "c4", "汽水、单车和傍晚的风。", "public", "published", "", "流行", `["流行","电子"]`, "", 30, 52000, 1204, 44, 97, "1 day"},
		{"w_noon", "t_zhouchi", "u_zhouchi", "无人接听", "c5", "打了很多次，都没有人接。", "public", "published", "", "R&B", `["R&B","情绪"]`, "", 60, 38000, 902, 31, 63, "1 day"},
		{"w_record", "t_gubei", "u_gubei", "旧唱片", "c6", "唱针落下的那一刻。", "public", "published", "", "爵士", `["爵士","Lo-fi"]`, "", 45, 61000, 1502, 57, 88, "2 days"},
		{"w_evening", "t_bailu", "u_bailu", "向晚", "c7", "日落之后，风慢下来。", "public", "published", "", "国风", `["国风","纯音乐"]`, "", 60, 99000, 2430, 74, 120, "3 days"},
		{"w_neon", "t_kai", "u_kai", "霓虹脉冲", "c8", "霓虹灯下的电子脉冲。", "public", "published", "", "电子", `["电子","摇滚"]`, "", 30, 173000, 4410, 210, 380, "4 days"},
	}
	for _, w := range works {
		if err := insertWork(w.id, w.tenant, w.owner, w.title, w.cover, w.desc, w.vis, w.status,
			w.reason, w.genre, w.tags, w.audio, w.dur, w.plays, w.likes, w.comments, w.shares, w.published); err != nil {
			return fmt.Errorf("seed work %s: %w", w.id, err)
		}
	}

	relations := [][2]string{
		{"u_self", "u_linan"}, {"u_linan", "u_self"},
		{"u_self", "u_alan"},
		{"u_self", "u_yiming"}, {"u_yiming", "u_self"},
		{"u_self", "u_xiaoye"}, {"u_xiaoye", "u_self"},
		{"u_self", "u_gubei"},
	}
	for _, r := range relations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO relations(follower_id, followee_id) VALUES($1,$2) ON CONFLICT DO NOTHING`,
			r[0], r[1]); err != nil {
			return err
		}
	}

	likes := [][2]string{
		{"u_self", "w_mist"}, {"u_self", "w_store"}, {"u_self", "w_unnamed"},
		{"u_linan", "w_night"}, {"u_alan", "w_night"}, {"u_yiming", "w_night"},
	}
	for _, l := range likes {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work_likes(work_id, user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, l[1], l[0]); err != nil {
			return err
		}
	}

	comments := []struct{ id, work, user, content string }{
		{"cm_1", "w_mist", "u_alan", "这段合成器太有画面感了，像雨后的高架桥。"},
		{"cm_2", "w_mist", "u_yiming", "30 秒循环听了一晚上，求完整版。"},
		{"cm_3", "w_mist", "u_xiaoye", "鼓点的呼吸感做得很好，是自己混的吗？"},
		{"cm_4", "w_mist", "u_bailu", "有电影片尾曲的感觉，收藏了。"},
		{"cm_5", "w_night", "u_linan", "副歌的空间感很好。"},
	}
	for _, c := range comments {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO comments(id, work_id, user_id, content) VALUES($1,$2,$3,$4)`,
			c.id, c.work, c.user, c.content); err != nil {
			return err
		}
	}

	notifications := []struct {
		id, user, typ, actor, title, body, targetType, targetID string
		unread                                                  bool
		offset                                                  string
	}{
		{"n_1", "u_self", "interaction", "u_linan", "林岸 关注了你", "", "user", "u_linan", true, "10 minutes"},
		{"n_2", "u_self", "interaction", "u_alan", "阿岚 等 12 人赞了你的作品《夜航》", "", "work", "w_night", true, "32 minutes"},
		{"n_3", "u_self", "work", "", "作品《夜航》已通过审核，正在广场展示", "", "work", "w_night", true, "1 hour"},
		{"n_4", "u_self", "work", "", "生成完成：《未命名作品 03》可以试听了", "", "work", "w_unnamed", false, "2 hours"},
		{"n_5", "u_self", "credit", "", "生成失败，已退还 32 积分", "", "generation", "g_seed", false, "1 day"},
		{"n_6", "u_self", "work", "", "作品《空白房间》未通过审核", "封面含未授权素材", "work", "w_blank", false, "1 day"},
		{"n_7", "u_self", "system", "", "版本更新：支持朋友可见作品与私信", "", "", "", false, "2 days"},
		{"n_8", "u_self", "interaction", "u_yiming", "陈一鸣 评论了你的作品：这段鼓点太舒服了", "", "work", "w_night", false, "3 days"},
	}
	for _, n := range notifications {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO notifications(id, user_id, type, actor_id, title, body, target_type, target_id, read_at, created_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,
				CASE WHEN $9::bool THEN NULL ELSE now() END,
				now()-$10::interval)`,
			n.id, n.user, n.typ, n.actor, n.title, n.body, n.targetType, n.targetID, n.unread, n.offset); err != nil {
			return err
		}
	}

	convRows := []struct{ id, a, b string }{
		{"cv_1", "u_self", "u_linan"},
		{"cv_2", "u_self", "u_alan"},
		{"cv_3", "u_self", "u_yiming"},
		{"cv_4", "u_self", "u_xiaoye"},
	}
	for i, c := range convRows {
		unread := 0
		if i == 0 {
			unread = 2
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO conversations(id, last_message_at) VALUES($1, now()-$2::interval)`,
			c.id, fmt.Sprintf("%d hours", i+1)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO conversation_members(conversation_id, user_id, unread_count, pinned)
			VALUES($1,$2,$3,$4),($1,$5,0,false)`,
			c.id, c.a, unread, i == 0, c.b); err != nil {
			return err
		}
	}

	messages := []struct {
		id, conv, sender, typ, text, work, offset string
	}{
		{"m_1", "cv_1", "u_linan", "text", "在广场听到你的《夜航》，编曲很干净。", "", "5 hours"},
		{"m_2", "cv_1", "u_self", "text", "谢谢！那首是凌晨写的，用了 Lo-fi 鼓组和一点环境音。", "", "4 hours"},
		{"m_3", "cv_1", "u_linan", "text", "那首《夜航》的鼓点我太喜欢了", "", "2 hours"},
		{"m_4", "cv_1", "u_linan", "song", "", "w_mist", "2 hours"},
		{"m_5", "cv_2", "u_alan", "text", "分享一首最近循环的歌给你。", "", "1 day"},
		{"m_6", "cv_2", "u_alan", "song", "", "w_evening", "1 day"},
		{"m_7", "cv_3", "u_yiming", "text", "你的主页我看了，作品都很稳。", "", "2 days"},
		{"m_8", "cv_4", "u_xiaoye", "text", "下次一起写一首？我最近在试 City Pop。", "", "3 days"},
	}
	for _, m := range messages {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO messages(id, conversation_id, sender_id, type, text, work_id, created_at)
			VALUES($1,$2,$3,$4,$5,$6,now()-$7::interval)`,
			m.id, m.conv, m.sender, m.typ, m.text, m.work, m.offset); err != nil {
			return err
		}
	}

	packages := []struct {
		id      string
		credits int
		price   int
		tag     string
		sort    int
	}{
		{"p_100", 60, 600, "", 1},
		{"p_300", 180, 1800, "", 2},
		{"p_680", 300, 3000, "最热", 3},
		{"p_1280", 580, 5800, "", 4},
	}
	for _, p := range packages {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO credit_packages(id, credits, price_cents, tag, sort_order)
			VALUES($1,$2,$3,$4,$5)`, p.id, p.credits, p.price, p.tag, p.sort); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO feed_policies(id, version, weights, diversity, status, created_by)
		VALUES('fp_1','v2026.11.06',
			'{"completion":35,"like":25,"share":20,"follow":20}'::jsonb,
			'{"author_gap":4,"genre_cap":3,"new_artist":1}'::jsonb,
			'active','u_self')`); err != nil {
		return err
	}

	reports := []struct{ id, reporter, targetType, targetID, reason, detail string }{
		{"rp_1", "u_bailu", "work", "w_neon", "疑似抄袭", "与另一首作品旋律相似"},
		{"rp_2", "u_self", "work", "w_blank", "侵权", "封面素材来源不明"},
		{"rp_3", "u_zhouchi", "work", "w_summer", "垃圾广告", "简介含联系方式"},
	}
	for _, r := range reports {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO reports(id, reporter_id, target_type, target_id, reason, detail)
			VALUES($1,$2,$3,$4,$5,$6)`, r.id, r.reporter, r.targetType, r.targetID, r.reason, r.detail); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO moderation_cases(id, target_type, target_id, decision, reason, status, reviewer_id, reviewed_at)
		VALUES('mc_seed','work','w_blank','reject','封面含未授权素材','closed','u_self',now())`); err != nil {
		return err
	}

	return tx.Commit()
}
