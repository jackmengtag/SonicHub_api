package seed

import (
	"context"
	"database/sql"
)

// SeedOps 补充后台运营模块的演示数据。与 Seed 不同，它在每次启动时执行，
// 使用固定主键与 ON CONFLICT DO NOTHING 保证幂等，可用于已有数据库。
func SeedOps(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 第二位管理员：用于演示积分调账的双人复核（申请人不可自审）。
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users(id, phone, display_name, avatar, bio, role, status, genres)
		VALUES('u_ops','13900000009','陆审核','c3','后台复核与内容审核。','admin','active','[]'::jsonb)
		ON CONFLICT DO NOTHING`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tenants(id, type, name, owner_user_id)
		VALUES('t_ops','personal','陆审核的空间','u_ops')
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO memberships(tenant_id, user_id, role) VALUES('t_ops','u_ops','owner')
		ON CONFLICT (tenant_id, user_id) DO NOTHING`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_accounts(tenant_id, available, reserved) VALUES('t_ops',128,0)
		ON CONFLICT (tenant_id) DO NOTHING`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
		VALUES('cl_grant_u_ops','t_ops','grant',128,128,'grant:u_ops','grant','u_ops')
		ON CONFLICT (idempotency_key) DO NOTHING`); err != nil {
		return err
	}

	// 审核员与运营账号：用于验证按角色生效的后台权限。
	staff := []struct {
		id, phone, name, avatar, role, tenant string
	}{
		{"u_reviewer", "13900000010", "陆晓", "c2", "reviewer", "t_reviewer"},
		{"u_operator", "13900000011", "周宁", "c4", "operator", "t_operator"},
	}
	for _, u := range staff {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO users(id, phone, display_name, avatar, bio, role, status, genres)
			VALUES($1,$2,$3,$4,'后台运营成员。',$5,'active','[]'::jsonb)
			ON CONFLICT DO NOTHING`, u.id, u.phone, u.name, u.avatar, u.role); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tenants(id, type, name, owner_user_id)
			VALUES($1,'personal',$2,$3) ON CONFLICT (id) DO NOTHING`,
			u.tenant, u.name+"的空间", u.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO memberships(tenant_id, user_id, role) VALUES($1,$2,'owner')
			ON CONFLICT (tenant_id, user_id) DO NOTHING`, u.tenant, u.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO credit_accounts(tenant_id, available, reserved) VALUES($1,128,0)
			ON CONFLICT (tenant_id) DO NOTHING`, u.tenant); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
			VALUES($1,$2,'grant',128,128,$3,'grant',$4)
			ON CONFLICT (idempotency_key) DO NOTHING`,
			"cl_grant_"+u.id, u.tenant, "grant:"+u.id, u.id); err != nil {
			return err
		}
	}

	adjustments := []struct {
		id, user, amount, kind, reason, requester, status string
	}{
		{"ca_seed_1", "u_xiaoye", "200", "客诉补偿", "生成失败导致重复扣费，按工单 CS-20261107-01 补发", "u_ops", "pending"},
		{"ca_seed_2", "u_zhouchi", "-50", "违规扣减", "作品封面命中违规素材，按规则扣减并已通知作者", "u_ops", "pending"},
		{"ca_seed_3", "u_linan", "300", "活动赠送", "城市声音征集活动奖励，工单 CS-20261105-08", "u_ops", "pending"},
	}
	for _, a := range adjustments {
		tenant := "t_" + a.user[2:]
		_, err := tx.ExecContext(ctx, `
			INSERT INTO credit_adjustments(id, tenant_id, user_id, amount, kind, reason, status, requester_id, reviewer_id, reviewed_at)
			VALUES($1,$2,$3,$4::bigint,$5,$6,$7,$8,
				CASE WHEN $7='approved' THEN 'u_self' ELSE '' END,
				CASE WHEN $7='approved' THEN now()-interval '1 day' ELSE NULL END)
			ON CONFLICT (id) DO NOTHING`,
			a.id, tenant, a.user, a.amount, a.kind, a.reason, a.status, a.requester)
		if err != nil {
			return err
		}
	}

	announcements := []struct {
		id, title, body, audience, channels, link, status, creator, sentOff string
	}{
		{"ann_seed_1", "冬日创作计划上线", "用 AI 写下你的冬天：完成主题创作即可获得额外积分与广场推荐位。", "all",
			`["站内","推送"]`, "/topics/winter", "sent", "u_self", "2 days"},
		{"ann_seed_2", "周末主题创作活动", "本周主题「深夜通勤」，欢迎把你的通勤故事写成歌。", "all",
			`["站内"]`, "/topics/commute", "draft", "u_self", ""},
		{"ann_seed_3", "音乐人认证升级说明", "认证材料与审核时效已更新，请在 7 日内补充代表作。", "creators",
			`["站内","推送"]`, "/settings/creator", "sent", "u_ops", "1 day"},
	}
	for _, a := range announcements {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO announcements(id, title, body, audience, channels, link, status, created_by, sent_at)
			VALUES($1,$2,$3,$4,$5::jsonb,$6,$7,$8,
				CASE WHEN $9='' THEN NULL ELSE now()-$9::interval END)
			ON CONFLICT (id) DO NOTHING`,
			a.id, a.title, a.body, a.audience, a.channels, a.link, a.status, a.creator, a.sentOff)
		if err != nil {
			return err
		}
	}

	templates := []struct {
		id, name, scene, channel, content string
		enabled                           bool
	}{
		{"tpl_seed_1", "生成完成通知", "写歌完成", "站内 + 推送", "你的作品《{title}》已生成完成，点击试听。", true},
		{"tpl_seed_2", "内容审核结果", "内容审核", "站内", "作品《{title}》审核{result}，原因：{reason}。", true},
		{"tpl_seed_3", "积分变动提醒", "积分变动", "站内", "你的积分{change} {amount}，当前余额 {balance}。", true},
		{"tpl_seed_4", "增长召回", "增长召回", "站内 + 短信", "好久不见，回来听听本周新歌。", false},
	}
	for _, t := range templates {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO message_templates(id, name, scene, channel, content, enabled)
			VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`,
			t.id, t.name, t.scene, t.channel, t.content, t.enabled); err != nil {
			return err
		}
	}

	placements := []struct {
		id, name, position, work, schedule, status, owner string
		exposure, clicks                                  int
	}{
		{"pl_seed_1", "冬日创作计划", "广场顶部 Banner", "w_mountain", "2026-11-01 ~ 2026-11-15", "online", "运营 · 周宁", 182300, 12480},
		{"pl_seed_2", "深夜 Lo-fi 电台", "推荐首页卡片", "w_store", "2026-11-05 ~ 2026-11-20", "online", "运营 · 周宁", 96400, 7330},
		{"pl_seed_3", "城市声音征集", "广场次卡位", "w_neon", "2026-11-10 ~ 2026-11-24", "scheduled", "内容运营 · 白露", 0, 0},
	}
	for _, p := range placements {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO plaza_placements(id, name, position, work_id, schedule, status, exposure, clicks, owner)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (id) DO NOTHING`,
			p.id, p.name, p.position, p.work, p.schedule, p.status, p.exposure, p.clicks, p.owner); err != nil {
			return err
		}
	}

	topics := []struct {
		id, name, desc, schedule, status string
		works, participants              int
	}{
		{"tp_seed_1", "#把冬天写成歌", "以冬天为主题创作，至少 30 秒。", "2026-11-01 ~ 2026-11-30", "online", 128, 342},
		{"tp_seed_2", "#深夜通勤歌单", "记录末班车与夜路的情绪。", "2026-11-05 ~ 2026-11-26", "online", 64, 221},
		{"tp_seed_3", "#AI 的第一首歌", "第一次用 AI 写歌的故事。", "2026-11-12 ~ 2026-11-30", "draft", 8, 36},
	}
	for _, t := range topics {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO plaza_topics(id, name, description, works_count, participants, schedule, status)
			VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO NOTHING`,
			t.id, t.name, t.desc, t.works, t.participants, t.schedule, t.status); err != nil {
			return err
		}
	}

	feedLists := []struct {
		id, listType, value string
	}{
		{"fl_seed_black_1", "black", "w_static"},
		{"fl_seed_black_2", "black", "低俗词库 v7"},
		{"fl_seed_black_3", "black", "竞品导流域名"},
		{"fl_seed_white_1", "white", "w_mountain"},
		{"fl_seed_white_2", "white", "w_snow"},
		{"fl_seed_white_3", "white", "官方活动作品"},
	}
	for _, f := range feedLists {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO feed_lists(id, list_type, value, created_by)
			VALUES($1,$2,$3,'u_self') ON CONFLICT (list_type, value) DO NOTHING`,
			f.id, f.listType, f.value); err != nil {
			return err
		}
	}

	fallbacks := []struct {
		id, work string
		order     int
	}{
		{"fb_seed_1", "w_mountain", 1},
		{"fb_seed_2", "w_night", 2},
		{"fb_seed_3", "w_evening", 3},
	}
	for _, f := range fallbacks {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO feed_fallbacks(id, work_id, sort_order, created_by)
			VALUES($1,$2,$3,'u_self') ON CONFLICT (work_id) DO NOTHING`,
			f.id, f.work, f.order); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE works SET is_pinned=true WHERE id='w_mountain' AND is_pinned=false`); err != nil {
		return err
	}

	return tx.Commit()
}
