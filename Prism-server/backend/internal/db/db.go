package db

import (
	"database/sql"
	"log"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

func Init(dbPath string) error {
	var err error
	DB, err = sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=cache_size=-20000")
	if err != nil {
		return err
	}
	DB.SetMaxOpenConns(12) // WAL: 多读 + 单写，busy_timeout 兜底写锁竞争
	DB.SetMaxIdleConns(12)
	DB.SetConnMaxLifetime(0) // 不设连接上限，避免频繁重建

	if err = migrate(); err != nil {
		return err
	}
	log.Printf("[DB] SQLite initialized: %s", dbPath)
	return nil
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}

func migrate() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			verified INTEGER NOT NULL DEFAULT 0,
			token TEXT UNIQUE NOT NULL,
			active_account_id INTEGER,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS game_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_id INTEGER REFERENCES users(id),
			display_name TEXT NOT NULL DEFAULT '',
			uid TEXT NOT NULL DEFAULT '',
			cookie_data TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'unknown',
			growth_level TEXT DEFAULT '',
			score TEXT DEFAULT '',
			skin_number TEXT DEFAULT '',
			cape_number TEXT DEFAULT '',
			is_vip INTEGER DEFAULT 0,
			source TEXT DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS verify_codes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL,
			username TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL DEFAULT '',
			code TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			used INTEGER NOT NULL DEFAULT 0
		)`,
		`ALTER TABLE verify_codes ADD COLUMN username TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE verify_codes ADD COLUMN password_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE game_accounts ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE game_accounts ADD COLUMN avatar_image_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE game_accounts ADD COLUMN skin_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE game_accounts ADD COLUMN created_by INTEGER`,
		`UPDATE game_accounts SET created_by = owner_id WHERE created_by IS NULL AND owner_id IS NOT NULL`,
		`ALTER TABLE users ADD COLUMN challenge_override INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN challenge_override_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN growth_override INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN growth_override_value INTEGER NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			account_id INTEGER,
			action TEXT NOT NULL,
			target TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS system_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			level TEXT NOT NULL DEFAULT 'info',
			message TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`ALTER TABLE verify_codes ADD COLUMN purpose TEXT NOT NULL DEFAULT 'register'`,
		`ALTER TABLE audit_logs ADD COLUMN token_id INTEGER`,
		`CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_logs(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action)`,
		`CREATE TABLE IF NOT EXISTS realname_presets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			id_number TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_accounts_owner ON game_accounts(owner_id)`,
		`ALTER TABLE users ADD COLUMN nuts_balance INTEGER NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS nuts_transactions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				amount INTEGER NOT NULL,
				reason TEXT NOT NULL,
				ref_user_id INTEGER,
				ref_account_id INTEGER,
				balance_after INTEGER NOT NULL,
				created_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS activation_codes (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				code TEXT UNIQUE NOT NULL,
				amount INTEGER NOT NULL,
				created_by INTEGER,
				created_at TEXT NOT NULL,
				redeemed_by INTEGER,
				redeemed_at TEXT
			)`,
		`CREATE TABLE IF NOT EXISTS red_packets (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				code TEXT UNIQUE NOT NULL,
				total INTEGER NOT NULL,
				remaining INTEGER NOT NULL,
				count INTEGER NOT NULL,
				remaining_count INTEGER NOT NULL,
				created_by INTEGER,
				created_by_type TEXT NOT NULL DEFAULT 'user',
				created_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS red_packet_claims (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				packet_id INTEGER NOT NULL,
				user_id INTEGER NOT NULL,
				amount INTEGER NOT NULL,
				claimed_at TEXT NOT NULL,
				UNIQUE(packet_id, user_id)
			)`,
		`CREATE TABLE IF NOT EXISTS skin_presets (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				item_id TEXT NOT NULL UNIQUE,
				preview_url TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS api_call_logs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER,
				endpoint TEXT NOT NULL,
				method TEXT NOT NULL DEFAULT '',
				success INTEGER NOT NULL DEFAULT 1,
				status_code INTEGER NOT NULL DEFAULT 200,
				duration_ms INTEGER NOT NULL DEFAULT 0,
				ip TEXT NOT NULL DEFAULT '',
				detail TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_api_calls_user_time ON api_call_logs(user_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_api_calls_ep_time ON api_call_logs(endpoint, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_api_calls_time ON api_call_logs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_api_calls_user_ep_time ON api_call_logs(user_id, endpoint, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_api_calls_ep_user_time ON api_call_logs(endpoint, user_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_user_action_time ON audit_logs(user_id, action, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_nuts_user_time ON nuts_transactions(user_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS payment_orders (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				order_no TEXT UNIQUE NOT NULL,
				trade_no TEXT NOT NULL DEFAULT '',
				user_id INTEGER NOT NULL,
				amount_yuan REAL NOT NULL,
				nuts_amount INTEGER NOT NULL,
				pay_type TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL DEFAULT 'pending',
				codes TEXT NOT NULL DEFAULT '[]',
				created_at TEXT NOT NULL,
				paid_at TEXT NOT NULL DEFAULT ''
			)`,
		`CREATE INDEX IF NOT EXISTS idx_payment_user ON payment_orders(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_payment_status ON payment_orders(status)`,
		`CREATE TABLE IF NOT EXISTS app_version_config (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				latest_version TEXT NOT NULL DEFAULT '',
				latest_version_code INTEGER NOT NULL DEFAULT 0,
				force_update INTEGER NOT NULL DEFAULT 0,
				update_url TEXT NOT NULL DEFAULT '',
				update_message TEXT NOT NULL DEFAULT '',
				updated_at TEXT NOT NULL DEFAULT ''
			)`,
		`CREATE TABLE IF NOT EXISTS app_announcements (
				id TEXT PRIMARY KEY,
				title TEXT NOT NULL,
				content TEXT NOT NULL,
				content_type TEXT NOT NULL DEFAULT 'markdown',
				severity TEXT NOT NULL DEFAULT 'info',
				display_mode TEXT NOT NULL DEFAULT 'modal',
				can_dismiss INTEGER NOT NULL DEFAULT 1,
				target_versions TEXT NOT NULL DEFAULT '[]',
				target_servers TEXT NOT NULL DEFAULT '[]',
				is_active INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL DEFAULT '',
				expires_at TEXT NOT NULL DEFAULT ''
			)`,
		`CREATE TABLE IF NOT EXISTS app_signatures (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				package_name TEXT NOT NULL,
				signature_hash TEXT NOT NULL,
				label TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL DEFAULT ''
			)`,
		`CREATE TABLE IF NOT EXISTS app_versions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				version TEXT NOT NULL,
				version_code INTEGER NOT NULL,
				release_notes TEXT NOT NULL DEFAULT '',
				file_name TEXT NOT NULL DEFAULT '',
				file_size INTEGER NOT NULL DEFAULT 0,
				is_current INTEGER NOT NULL DEFAULT 0,
				uploaded_at TEXT NOT NULL DEFAULT ''
			)`,
		`CREATE TABLE IF NOT EXISTS user_checkins (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				checkin_date TEXT NOT NULL,
				streak INTEGER NOT NULL,
				reward_nuts INTEGER NOT NULL,
				created_at TEXT NOT NULL,
				UNIQUE(user_id, checkin_date)
			)`,
		`CREATE TABLE IF NOT EXISTS invite_records (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				inviter_id INTEGER NOT NULL,
				invitee_id INTEGER NOT NULL,
				reward_type TEXT NOT NULL,
				reward_nuts INTEGER NOT NULL,
				created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_invite_inviter ON invite_records(inviter_id)`,
		`CREATE TABLE IF NOT EXISTS surveys (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				title TEXT NOT NULL,
				reward_nuts INTEGER NOT NULL DEFAULT 0,
				is_active INTEGER NOT NULL DEFAULT 0,
				has_answers INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS survey_questions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				survey_id INTEGER NOT NULL REFERENCES surveys(id) ON DELETE CASCADE,
				question_text TEXT NOT NULL,
				question_type TEXT NOT NULL DEFAULT 'choice',
				sort_order INTEGER NOT NULL DEFAULT 0,
				options TEXT NOT NULL DEFAULT '[]',
				created_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS survey_answers (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				survey_id INTEGER NOT NULL,
				question_id INTEGER NOT NULL,
				user_id INTEGER NOT NULL,
				answer_text TEXT NOT NULL,
				created_at TEXT NOT NULL,
				UNIQUE(survey_id, user_id, question_id)
			)`,
		`CREATE INDEX IF NOT EXISTS idx_survey_questions_survey ON survey_questions(survey_id, sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_survey_answers_survey_user ON survey_answers(survey_id, user_id)`,
		`CREATE TABLE IF NOT EXISTS toolbox_user_config (
				user_id INTEGER PRIMARY KEY,
				enabled INTEGER NOT NULL DEFAULT 0,
				expires_at TEXT NOT NULL DEFAULT '',
				prompt_purchased INTEGER NOT NULL DEFAULT 0,
				name_purchased INTEGER NOT NULL DEFAULT 0,
				custom_prompt TEXT NOT NULL DEFAULT '',
				custom_name TEXT NOT NULL DEFAULT '',
				trial_used INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS toolbox_defaults (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				default_prompt TEXT NOT NULL DEFAULT '',
				default_name TEXT NOT NULL DEFAULT '',
				updated_at TEXT NOT NULL DEFAULT ''
			)`,
		`CREATE TABLE IF NOT EXISTS api_tokens (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL REFERENCES users(id),
				name TEXT NOT NULL DEFAULT '',
				token TEXT UNIQUE NOT NULL,
				max_calls INTEGER NOT NULL DEFAULT -1,
				max_nuts INTEGER NOT NULL DEFAULT -1,
				call_count INTEGER NOT NULL DEFAULT 0,
				nuts_consumed INTEGER NOT NULL DEFAULT 0,
				disabled INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens(user_id)`,
		`ALTER TABLE api_call_logs ADD COLUMN api_token_id INTEGER`,
		`ALTER TABLE api_call_logs ADD COLUMN api_token_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE api_tokens ADD COLUMN active_account_id INTEGER`,
		`ALTER TABLE activation_codes ADD COLUMN source TEXT NOT NULL DEFAULT 'admin'`,
		`ALTER TABLE activation_codes ADD COLUMN question TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE activation_codes ADD COLUMN answer_hash TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_red_packets_created ON red_packets(created_by)`,
		`CREATE INDEX IF NOT EXISTS idx_rpc_packet_user ON red_packet_claims(packet_id, user_id)`,
		// ── MC 建筑文件市场 ──
		`CREATE TABLE IF NOT EXISTS market_upload_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tts_hash TEXT UNIQUE NOT NULL,
			user_id INTEGER NOT NULL,
			file_md5 TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			previews_md5 TEXT NOT NULL DEFAULT '[]',
			price_pts INTEGER NOT NULL DEFAULT 0,
			expires_at TEXT NOT NULL,
			used INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS market_files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uploader_id INTEGER NOT NULL REFERENCES users(id),
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '',
			tags TEXT NOT NULL DEFAULT '[]',
			price_pts INTEGER NOT NULL DEFAULT 0,
			allow_anonymous INTEGER NOT NULL DEFAULT 1,
			md5 TEXT NOT NULL,
			block_count INTEGER NOT NULL DEFAULT 0,
			nbt_count INTEGER NOT NULL DEFAULT 0,
			file_size INTEGER NOT NULL DEFAULT 0,
			block_dims TEXT NOT NULL DEFAULT '{}',
			preview_cam TEXT NOT NULL DEFAULT '{}',
			download_count INTEGER NOT NULL DEFAULT 0,
			cover_part_id INTEGER NOT NULL DEFAULT 0,
			part_dir TEXT NOT NULL,
			flagged INTEGER NOT NULL DEFAULT 0,
			report_count INTEGER NOT NULL DEFAULT 0,
			status INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_market_files_md5 ON market_files(md5)`,
		`CREATE INDEX IF NOT EXISTS idx_market_files_cat ON market_files(category, status)`,
		`CREATE INDEX IF NOT EXISTS idx_market_files_uploader ON market_files(uploader_id)`,
		`CREATE TABLE IF NOT EXISTS market_file_parts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES market_files(id) ON DELETE CASCADE,
			part_type TEXT NOT NULL,
			orig_name TEXT NOT NULL,
			stored_name TEXT NOT NULL,
			ext TEXT NOT NULL,
			rel_path TEXT NOT NULL,
			md5 TEXT NOT NULL DEFAULT '',
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			size INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_market_parts_file ON market_file_parts(file_id)`,
		`CREATE TABLE IF NOT EXISTS market_downloads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES market_files(id),
			user_id INTEGER NOT NULL,
			price INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			UNIQUE(file_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_market_downloads_file ON market_downloads(file_id)`,
		`CREATE TABLE IF NOT EXISTS market_reports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES market_files(id),
			reporter_id INTEGER NOT NULL REFERENCES users(id),
			reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(file_id, reporter_id)
		)`,
		`CREATE TABLE IF NOT EXISTS market_config (
			id INTEGER PRIMARY KEY,
			upload_reward_nuts INTEGER NOT NULL DEFAULT 10,
			report_auto_takedown INTEGER NOT NULL DEFAULT 3,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS market_categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS market_file_categories (
			file_id INTEGER NOT NULL REFERENCES market_files(id) ON DELETE CASCADE,
			category_id INTEGER NOT NULL REFERENCES market_categories(id) ON DELETE CASCADE,
			PRIMARY KEY(file_id, category_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mfc_cat ON market_file_categories(category_id)`,
		// 市场：为已有库补列（新库由上面的 CREATE 自带；重复列被循环忽略）
		`ALTER TABLE market_files ADD COLUMN nbt_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE market_files ADD COLUMN file_size INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE market_files ADD COLUMN cover_part_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE market_upload_sessions ADD COLUMN block_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE market_upload_sessions ADD COLUMN nbt_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE market_upload_sessions ADD COLUMN block_dims TEXT NOT NULL DEFAULT '{}'`,
		// 市场：文件评论（多级回复树，parent_id=0 为根评论）
		`CREATE TABLE IF NOT EXISTS market_comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES market_files(id) ON DELETE CASCADE,
			parent_id INTEGER NOT NULL DEFAULT 0,
			user_id INTEGER NOT NULL REFERENCES users(id),
			content TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mc_file ON market_comments(file_id)`,
		// 默认分类 seed（幂等）
		`INSERT OR IGNORE INTO market_categories (name, sort_order) VALUES ('现代建筑', 1)`,
		`INSERT OR IGNORE INTO market_categories (name, sort_order) VALUES ('中世纪', 2)`,
		`INSERT OR IGNORE INTO market_categories (name, sort_order) VALUES ('中式', 3)`,
		`INSERT OR IGNORE INTO market_categories (name, sort_order) VALUES ('现代别墅', 4)`,
		`INSERT OR IGNORE INTO market_categories (name, sort_order) VALUES ('景观', 5)`,
		`INSERT OR IGNORE INTO market_categories (name, sort_order) VALUES ('红石机械', 6)`,
		`INSERT OR IGNORE INTO market_categories (name, sort_order) VALUES ('其他', 7)`,
		// 推送通知：管理员推送消息 + 用户已读记录（幂等，旧库补列会被循环忽略）
		`CREATE TABLE IF NOT EXISTS push_messages (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT '',
			action_data TEXT NOT NULL DEFAULT '',
			display_mode TEXT NOT NULL DEFAULT '',
			target_user_id INTEGER,
			target_version TEXT NOT NULL DEFAULT '',
			priority INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS user_push_reads (
			user_id INTEGER NOT NULL,
			msg_id TEXT NOT NULL,
			read_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (user_id, msg_id)
		)`,
		// 插件市场：上传会话 / 文件（列表）/ 版本 / parts / 下载 / 举报 / 评论 / 分类 / 配置
		`CREATE TABLE IF NOT EXISTS plugin_market_upload_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tts_hash TEXT UNIQUE NOT NULL,
			user_id INTEGER NOT NULL,
			file_md5 TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			price_pts INTEGER NOT NULL DEFAULT 0,
			expires_at TEXT NOT NULL,
			used INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS plugin_market_files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plugin_id TEXT NOT NULL,
			uploader_id INTEGER NOT NULL REFERENCES users(id),
			name TEXT NOT NULL,
			version TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT 'lua',
			description TEXT NOT NULL DEFAULT '',
			docs TEXT NOT NULL DEFAULT '',
			categories TEXT NOT NULL DEFAULT '[]',
			tags TEXT NOT NULL DEFAULT '[]',
			declared_caps TEXT NOT NULL DEFAULT '[]',
			scan_result TEXT NOT NULL DEFAULT '[]',
			risk_level INTEGER NOT NULL DEFAULT 0,
			price_pts INTEGER NOT NULL DEFAULT 0,
			allow_anonymous INTEGER NOT NULL DEFAULT 1,
			card_color TEXT NOT NULL DEFAULT '',
			zip_hash TEXT NOT NULL DEFAULT '',
			norm_hash TEXT NOT NULL DEFAULT '',
			md5 TEXT NOT NULL DEFAULT '',
			file_size INTEGER NOT NULL DEFAULT 0,
			download_count INTEGER NOT NULL DEFAULT 0,
			review_state INTEGER NOT NULL DEFAULT 0,
			dedup_flag INTEGER NOT NULL DEFAULT 0,
			flag_reason TEXT NOT NULL DEFAULT '',
			cover_part_id INTEGER NOT NULL DEFAULT 0,
			part_dir TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pmf_plugin ON plugin_market_files(plugin_id, uploader_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pmf_norm ON plugin_market_files(norm_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_pmf_state ON plugin_market_files(review_state)`,
		`CREATE TABLE IF NOT EXISTS plugin_market_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES plugin_market_files(id) ON DELETE CASCADE,
			version TEXT NOT NULL DEFAULT '',
			changelog TEXT NOT NULL DEFAULT '',
			zip_part_id INTEGER NOT NULL DEFAULT 0,
			zip_hash TEXT NOT NULL DEFAULT '',
			norm_hash TEXT NOT NULL DEFAULT '',
			md5 TEXT NOT NULL DEFAULT '',
			file_size INTEGER NOT NULL DEFAULT 0,
			review_state INTEGER NOT NULL DEFAULT 0,
			dedup_flag INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pmv_file ON plugin_market_versions(file_id)`,
		`CREATE TABLE IF NOT EXISTS plugin_market_parts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES plugin_market_files(id) ON DELETE CASCADE,
			part_type TEXT NOT NULL,
			orig_name TEXT NOT NULL,
			stored_name TEXT NOT NULL,
			ext TEXT NOT NULL,
			rel_path TEXT NOT NULL,
			md5 TEXT NOT NULL DEFAULT '',
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			size INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pmp_file ON plugin_market_parts(file_id)`,
		`CREATE TABLE IF NOT EXISTS plugin_market_downloads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES plugin_market_files(id),
			user_id INTEGER NOT NULL,
			price INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			UNIQUE(file_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS plugin_market_reports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES plugin_market_files(id),
			reporter_id INTEGER NOT NULL REFERENCES users(id),
			reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(file_id, reporter_id)
		)`,
		`CREATE TABLE IF NOT EXISTS plugin_market_comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES plugin_market_files(id) ON DELETE CASCADE,
			parent_id INTEGER NOT NULL DEFAULT 0,
			user_id INTEGER NOT NULL REFERENCES users(id),
			content TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS plugin_market_categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS plugin_market_file_categories (
			file_id INTEGER NOT NULL REFERENCES plugin_market_files(id) ON DELETE CASCADE,
			category_id INTEGER NOT NULL REFERENCES plugin_market_categories(id) ON DELETE CASCADE,
			PRIMARY KEY(file_id, category_id)
		)`,
		`CREATE TABLE IF NOT EXISTS plugin_market_config (
			id INTEGER PRIMARY KEY,
			upload_reward_nuts INTEGER NOT NULL DEFAULT 10,
			report_auto_takedown INTEGER NOT NULL DEFAULT 3,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`INSERT OR IGNORE INTO plugin_market_categories (name, sort_order) VALUES ('工具辅助', 1)`,
		`INSERT OR IGNORE INTO plugin_market_categories (name, sort_order) VALUES ('生存玩法', 2)`,
		`INSERT OR IGNORE INTO plugin_market_categories (name, sort_order) VALUES ('建筑辅助', 3)`,
		`INSERT OR IGNORE INTO plugin_market_categories (name, sort_order) VALUES ('娱乐', 4)`,
		`INSERT OR IGNORE INTO plugin_market_categories (name, sort_order) VALUES ('其他', 5)`,
		// 旧库的 push_messages 表可能已存在但缺 display_mode 列；重复列会被循环忽略
		`ALTER TABLE push_messages ADD COLUMN display_mode TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN subscription_start TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN subscription_until TEXT NOT NULL DEFAULT ''`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return err
		}
	}
	tx.Exec(`ALTER TABLE game_accounts ADD COLUMN growth_exp TEXT NOT NULL DEFAULT ''`)
	tx.Exec(`ALTER TABLE game_accounts ADD COLUMN growth_need_exp TEXT NOT NULL DEFAULT ''`)
	tx.Exec(`ALTER TABLE game_accounts ADD COLUMN auto_refresh_enabled INTEGER NOT NULL DEFAULT 0`)
	tx.Exec(`ALTER TABLE game_accounts ADD COLUMN is_server_owner INTEGER NOT NULL DEFAULT 0`)
	// Enable auto-refresh for cookie/guest accounts, disabled for phone/email
	tx.Exec(`UPDATE game_accounts SET auto_refresh_enabled = 1 WHERE source IN ('cookie','guest','','web') OR source IS NULL`)
	tx.Exec(`ALTER TABLE users ADD COLUMN activated INTEGER NOT NULL DEFAULT 1`)
	tx.Exec(`ALTER TABLE users ADD COLUMN login_count INTEGER NOT NULL DEFAULT 0`)
	tx.Exec(`CREATE TABLE IF NOT EXISTS activation_keys (id INTEGER PRIMARY KEY AUTOINCREMENT, code TEXT UNIQUE NOT NULL, note TEXT DEFAULT '', created_at TEXT NOT NULL)`)
	tx.Exec(`INSERT OR IGNORE INTO activation_keys (code, note, created_at) VALUES ('6euqj5', '默认激活码', datetime('now'))`)
	tx.Exec(`ALTER TABLE realname_presets ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`)
	tx.Exec(`ALTER TABLE verify_codes ADD COLUMN invite_code TEXT NOT NULL DEFAULT ''`)
	tx.Exec(`ALTER TABLE users ADD COLUMN invite_code TEXT NOT NULL DEFAULT ''`)
	tx.Exec(`ALTER TABLE users ADD COLUMN invited_by INTEGER`)
	tx.Exec(`ALTER TABLE users ADD COLUMN test_account INTEGER NOT NULL DEFAULT 0`)
	// Generate invite codes for existing users
	tx.Exec(`UPDATE users SET invite_code = CAST(ABS(RANDOM()) % 900000 + 100000 AS TEXT) WHERE invite_code = '' OR invite_code IS NULL`)
	// Seed default realname preset
	if _, err := tx.Exec(`INSERT OR IGNORE INTO realname_presets (id, name, id_number, enabled, created_at) VALUES (1, '赵孟岭', '130529198702185333', 1, ?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}

	// ── P2P 节点归属（令牌 affinity 系统）──
	tx.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token_id INTEGER,
		is_master INTEGER NOT NULL DEFAULT 0,
		tunnel_id TEXT NOT NULL UNIQUE,
		socks_port INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'online',
		last_seen TEXT,
		created_at TEXT NOT NULL
	)`)
	tx.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_user ON nodes(user_id)`)

	return tx.Commit()
}
