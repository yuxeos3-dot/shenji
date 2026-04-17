package api

// One-time migration from global platform settings to per-user storage.
//
// Before this refactor, the system had exactly one Telegram bot, one
// Bing API key, one FRW apikey, and one OAuth token row per provider.
// The launch architecture is "every user is their own system" — each
// employee manages their own ~5 sites and brings their own platform
// credentials. We migrate the existing global rows onto a designated
// user (default: 沈万三 / shenwanshan@gmail.com, falls back to the first
// admin if that account isn't present) so current connections stay
// live, then delete the global keys.
//
// Runs exactly once — writes `perm_migrated_personal_v1` flag to
// settings when done. Other users whose data is already bound to this
// person simply need to re-fill their own credentials via the new
// 个人中心 page. That's the intended outcome: private onboarding per
// user, no shared blob.

import (
	"database/sql"
	"log"

	"seo-system/internal/db"
)

// MigrateGlobalToPersonalConfig copies global platform settings to a
// per-user record on the target user. Idempotent — sets
// `perm_migrated_personal_v1=1` when complete and no-ops on every
// subsequent boot.
//
// Wrapped in a top-level recover so a migration bug on production data
// (e.g. an unexpected NULL in a pre-DEFAULT-era row) can't crash-loop
// the whole service at startup. We log the panic and continue; admin
// can investigate via logs and re-run after fixing the data.
func MigrateGlobalToPersonalConfig() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[personal-migration] PANIC: %v (service continues; retry on next boot after DB cleanup)", r)
		}
	}()

	var done string
	db.DB.QueryRow(`SELECT COALESCE(value,'') FROM settings WHERE key='perm_migrated_personal_v1'`).Scan(&done)
	if done == "1" {
		return
	}

	target := resolveMigrationTargetUser()
	if target <= 0 {
		log.Println("[personal-migration] no target user found; skipping, will retry next boot")
		return
	}

	migrated := 0
	// ─── OAuth app credentials (GSC + GA share one; Yandex separate) ───
	if cid := db.GetSetting("gsc_client_id"); cid != "" {
		cs := db.GetSetting("gsc_client_secret")
		ru := db.GetSetting("gsc_redirect_uri")
		encSecret := ""
		if cs != "" {
			if v, err := EncryptToken(cs); err == nil {
				encSecret = v
			}
		}
		_, err := db.DB.Exec(
			`INSERT INTO user_oauth_config (user_id, service, client_id, client_secret, redirect_uri)
			 VALUES (?, 'gsc', ?, ?, ?)
			 ON CONFLICT(user_id, service) DO UPDATE SET
			   client_id = excluded.client_id,
			   client_secret = CASE WHEN excluded.client_secret != '' THEN excluded.client_secret ELSE user_oauth_config.client_secret END,
			   redirect_uri = excluded.redirect_uri,
			   updated_at = CURRENT_TIMESTAMP`,
			target, cid, encSecret, ru,
		)
		if err != nil {
			log.Printf("[personal-migration] gsc config: %v", err)
		} else {
			// Google Analytics shares the same OAuth app — seed a GA row too.
			gaRU := ru
			// The existing code swaps "/gsc/" → "/ga/" in redirect URIs.
			if gaRU != "" {
				gaRU = replaceServicePath(gaRU, "/gsc/", "/ga/")
			}
			if _, err := db.DB.Exec(
				`INSERT INTO user_oauth_config (user_id, service, client_id, client_secret, redirect_uri)
				 VALUES (?, 'ga', ?, ?, ?)
				 ON CONFLICT(user_id, service) DO UPDATE SET
				   client_id = excluded.client_id,
				   client_secret = CASE WHEN excluded.client_secret != '' THEN excluded.client_secret ELSE user_oauth_config.client_secret END,
				   redirect_uri = excluded.redirect_uri,
				   updated_at = CURRENT_TIMESTAMP`,
				target, cid, encSecret, gaRU,
			); err != nil {
				log.Printf("[personal-migration] ga config: %v", err)
			}
			migrated++
		}
	}
	if cid := db.GetSetting("yandex_client_id"); cid != "" {
		cs := db.GetSetting("yandex_client_secret")
		ru := db.GetSetting("yandex_redirect_uri")
		encSecret := ""
		if cs != "" {
			if v, err := EncryptToken(cs); err == nil {
				encSecret = v
			}
		}
		if _, err := db.DB.Exec(
			`INSERT INTO user_oauth_config (user_id, service, client_id, client_secret, redirect_uri)
			 VALUES (?, 'yandex', ?, ?, ?)
			 ON CONFLICT(user_id, service) DO UPDATE SET
			   client_id = excluded.client_id,
			   client_secret = CASE WHEN excluded.client_secret != '' THEN excluded.client_secret ELSE user_oauth_config.client_secret END,
			   redirect_uri = excluded.redirect_uri,
			   updated_at = CURRENT_TIMESTAMP`,
			target, cid, encSecret, ru,
		); err != nil {
			log.Printf("[personal-migration] yandex config: %v", err)
		} else {
			migrated++
		}
	}

	// ─── OAuth access tokens ────────────────────────────────────────────
	// Existing rows in oauth_tokens (global) are re-associated with the
	// target user. Values are already encrypted — copy as-is. Use
	// nullable scan targets because pre-DEFAULT-clause rows can still
	// carry NULLs for refresh_token / expires_at, and `string` Scan
	// would fail on NULL.
	rows, err := db.DB.Query(
		`SELECT service, COALESCE(access_token,''), COALESCE(refresh_token,''), expires_at
		 FROM oauth_tokens ORDER BY id DESC`,
	)
	if err == nil {
		defer rows.Close()
		seen := map[string]bool{}
		for rows.Next() {
			var svc, at, rt string
			var expiresAt sql.NullTime
			if err := rows.Scan(&svc, &at, &rt, &expiresAt); err != nil {
				log.Printf("[personal-migration] oauth_tokens scan: %v", err)
				continue
			}
			if seen[svc] {
				continue // keep only most-recent per service
			}
			seen[svc] = true
			if at == "" {
				continue // nothing useful to migrate
			}
			var expiresVal interface{}
			if expiresAt.Valid {
				expiresVal = expiresAt.Time
			}
			if _, err := db.DB.Exec(
				`INSERT INTO user_oauth_tokens (user_id, service, access_token, refresh_token, expires_at)
				 VALUES (?, ?, ?, ?, ?)
				 ON CONFLICT(user_id, service) DO UPDATE SET
				   access_token = excluded.access_token,
				   refresh_token = excluded.refresh_token,
				   expires_at = excluded.expires_at,
				   updated_at = CURRENT_TIMESTAMP`,
				target, svc, at, rt, expiresVal,
			); err != nil {
				log.Printf("[personal-migration] token %s: %v", svc, err)
			} else {
				migrated++
			}
		}
	}

	// ─── Telegram ───────────────────────────────────────────────────────
	if tok := db.GetSetting("telegram_bot_token"); tok != "" {
		chat := db.GetSetting("telegram_chat_id")
		enc := tok
		// Global tokens were stored plaintext — encrypt on the way in.
		if v, err := EncryptToken(tok); err == nil {
			enc = v
		}
		if _, err := db.DB.Exec(
			`INSERT INTO user_telegram (user_id, bot_token, chat_id) VALUES (?, ?, ?)
			 ON CONFLICT(user_id) DO UPDATE SET
			   bot_token = excluded.bot_token,
			   chat_id = excluded.chat_id,
			   updated_at = CURRENT_TIMESTAMP`,
			target, enc, chat,
		); err != nil {
			log.Printf("[personal-migration] telegram: %v", err)
		} else {
			migrated++
		}
	}

	// ─── Bing API key ───────────────────────────────────────────────────
	if key := db.GetSetting("bing_api_key"); key != "" {
		enc := key
		if v, err := EncryptToken(key); err == nil {
			enc = v
		}
		if _, err := db.DB.Exec(
			`INSERT INTO user_bing (user_id, api_key) VALUES (?, ?)
			 ON CONFLICT(user_id) DO UPDATE SET
			   api_key = excluded.api_key,
			   updated_at = CURRENT_TIMESTAMP`,
			target, enc,
		); err != nil {
			log.Printf("[personal-migration] bing: %v", err)
		} else {
			migrated++
		}
	}

	// ─── FRW apikey ─────────────────────────────────────────────────────
	if key := db.GetSetting("frw_apikey"); key != "" {
		enc := key
		if v, err := EncryptToken(key); err == nil {
			enc = v
		}
		if _, err := db.DB.Exec(`UPDATE users SET frw_apikey = ? WHERE id = ?`, enc, target); err != nil {
			log.Printf("[personal-migration] frw: %v", err)
		} else {
			migrated++
		}
	}

	// ─── Delete the migrated keys from settings ────────────────────────
	// ai_* keys also get removed — AI config is already fully per-user
	// (users.ai_slots) so any residual global AI setting is dead weight.
	obsoleteKeys := []string{
		"gsc_client_id", "gsc_client_secret", "gsc_redirect_uri",
		"yandex_client_id", "yandex_client_secret", "yandex_redirect_uri",
		"bing_api_key",
		"telegram_bot_token", "telegram_chat_id",
		"frw_apikey",
		"ai_api_key", "ai_api_url", "ai_model",
	}
	for _, k := range obsoleteKeys {
		if _, err := db.DB.Exec(`DELETE FROM settings WHERE key = ?`, k); err != nil {
			log.Printf("[personal-migration] delete setting %s: %v", k, err)
		}
	}
	// Old global token rows are also cleared — user_oauth_tokens is the
	// authoritative source going forward.
	if _, err := db.DB.Exec(`DELETE FROM oauth_tokens`); err != nil {
		log.Printf("[personal-migration] delete oauth_tokens: %v", err)
	}

	db.DB.Exec(
		`INSERT INTO settings(key, value) VALUES('perm_migrated_personal_v1','1')
		 ON CONFLICT(key) DO UPDATE SET value='1'`,
	)

	log.Printf("[personal-migration] migrated %d global credential(s) to user id=%d", migrated, target)
}

// resolveMigrationTargetUser returns 沈万三's user id, or the first admin
// if that account doesn't exist. The function deliberately hard-codes the
// email rather than reading it from env — this only runs once ever, and
// the correct target is part of the launch agreement.
func resolveMigrationTargetUser() int64 {
	var id int64
	err := db.DB.QueryRow(
		`SELECT id FROM users WHERE LOWER(email) = 'shenwanshan@gmail.com' LIMIT 1`,
	).Scan(&id)
	if err == nil && id > 0 {
		log.Printf("[personal-migration] target: 沈万三 (id=%d)", id)
		return id
	}
	// Fallback: first admin.
	err = db.DB.QueryRow(
		`SELECT id FROM users WHERE role = 'admin' ORDER BY id ASC LIMIT 1`,
	).Scan(&id)
	if err == nil && id > 0 {
		log.Printf("[personal-migration] fallback target: first admin (id=%d)", id)
		return id
	}
	return 0
}

// replaceServicePath swaps a path segment (e.g. "/gsc/" → "/ga/") in a URL
// without pulling in url.Parse just for this. Keeps query string intact.
func replaceServicePath(raw, from, to string) string {
	out := raw
	for {
		i := indexOfCI(out, from)
		if i < 0 {
			return out
		}
		out = out[:i] + to + out[i+len(from):]
	}
}

// indexOfCI is a case-insensitive substring search. Small helper so we
// avoid a dependency just to avoid writing three lines.
func indexOfCI(s, substr string) int {
	if substr == "" {
		return 0
	}
	ls := len(s)
	lb := len(substr)
	if lb > ls {
		return -1
	}
	for i := 0; i <= ls-lb; i++ {
		match := true
		for j := 0; j < lb; j++ {
			a := s[i+j]
			b := substr[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
