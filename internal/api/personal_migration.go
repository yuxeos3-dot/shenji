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
//
// The migration runs in a BACKGROUND GOROUTINE from main() so even if
// it deadlocks or takes a long time, the HTTP server comes up and
// login keeps working. On a single-connection SQLite pool
// (MaxOpenConns=1), any Exec issued while a Rows iterator is still
// open deadlocks forever — this file is careful to drain Rows into a
// slice and Close BEFORE issuing any follow-up writes.

import (
	"database/sql"
	"log"
	"os"

	"seo-system/internal/db"
)

// MigrateGlobalToPersonalConfig copies global platform settings to a
// per-user record on the target user. Idempotent — sets
// `perm_migrated_personal_v1=1` when complete and no-ops on every
// subsequent boot.
func MigrateGlobalToPersonalConfig() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[personal-migration] PANIC: %v (service unaffected; retry next boot)", r)
		}
	}()

	var done string
	db.DB.QueryRow(`SELECT COALESCE(value,'') FROM settings WHERE key='perm_migrated_personal_v1'`).Scan(&done)
	if done == "1" {
		log.Println("[personal-migration] already completed, skipping")
		return
	}

	target := resolveMigrationTargetUser()
	if target <= 0 {
		log.Println("[personal-migration] no target user found; skipping, will retry next boot")
		return
	}

	migrated := 0

	// ─── OAuth app credentials (GSC + GA share one; Yandex separate) ───
	log.Println("[personal-migration] step 1/5: OAuth app credentials")
	if cid := db.GetSetting("gsc_client_id"); cid != "" {
		cs := db.GetSetting("gsc_client_secret")
		ru := db.GetSetting("gsc_redirect_uri")
		encSecret := ""
		if cs != "" {
			if v, err := EncryptToken(cs); err == nil {
				encSecret = v
			}
		}
		if _, err := db.DB.Exec(
			`INSERT INTO user_oauth_config (user_id, service, client_id, client_secret, redirect_uri)
			 VALUES (?, 'gsc', ?, ?, ?)
			 ON CONFLICT(user_id, service) DO UPDATE SET
			   client_id = excluded.client_id,
			   client_secret = CASE WHEN excluded.client_secret != '' THEN excluded.client_secret ELSE user_oauth_config.client_secret END,
			   redirect_uri = excluded.redirect_uri,
			   updated_at = CURRENT_TIMESTAMP`,
			target, cid, encSecret, ru,
		); err != nil {
			log.Printf("[personal-migration] gsc config: %v", err)
		} else {
			// Google Analytics shares the same OAuth app — seed a GA row too.
			gaRU := ru
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
	// CRITICAL: read all rows fully + close BEFORE issuing any write. On
	// modernc.org/sqlite with MaxOpenConns=1 the single connection is
	// pinned to the Rows iterator; issuing db.DB.Exec inside the rows.Next
	// loop deadlocks forever (Exec waits for conn, Rows never releases
	// until Close). Drain into a slice, Close, then INSERT.
	log.Println("[personal-migration] step 2/5: OAuth access tokens")
	type tokRow struct {
		svc, at, rt string
		expiresAt   sql.NullTime
	}
	var toks []tokRow
	if rows, err := db.DB.Query(
		`SELECT service, COALESCE(access_token,''), COALESCE(refresh_token,''), expires_at
		 FROM oauth_tokens ORDER BY id DESC`,
	); err == nil {
		seen := map[string]bool{}
		for rows.Next() {
			var r tokRow
			if err := rows.Scan(&r.svc, &r.at, &r.rt, &r.expiresAt); err != nil {
				log.Printf("[personal-migration] oauth_tokens scan: %v", err)
				continue
			}
			if seen[r.svc] || r.at == "" {
				continue
			}
			seen[r.svc] = true
			toks = append(toks, r)
		}
		rows.Close()
	}
	for _, r := range toks {
		var expiresVal interface{}
		if r.expiresAt.Valid {
			expiresVal = r.expiresAt.Time
		}
		if _, err := db.DB.Exec(
			`INSERT INTO user_oauth_tokens (user_id, service, access_token, refresh_token, expires_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(user_id, service) DO UPDATE SET
			   access_token = excluded.access_token,
			   refresh_token = excluded.refresh_token,
			   expires_at = excluded.expires_at,
			   updated_at = CURRENT_TIMESTAMP`,
			target, r.svc, r.at, r.rt, expiresVal,
		); err != nil {
			log.Printf("[personal-migration] token %s: %v", r.svc, err)
		} else {
			migrated++
		}
	}

	// ─── Telegram ───────────────────────────────────────────────────────
	log.Println("[personal-migration] step 3/5: Telegram bot")
	if tok := db.GetSetting("telegram_bot_token"); tok != "" {
		chat := db.GetSetting("telegram_chat_id")
		enc := tok
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
	log.Println("[personal-migration] step 4/5: Bing + FRW")
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
	log.Println("[personal-migration] step 5/5: cleaning up global keys")
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
	if _, err := db.DB.Exec(`DELETE FROM oauth_tokens`); err != nil {
		log.Printf("[personal-migration] delete oauth_tokens: %v", err)
	}

	if _, err := db.DB.Exec(
		`INSERT INTO settings(key, value) VALUES('perm_migrated_personal_v1','1')
		 ON CONFLICT(key) DO UPDATE SET value='1'`,
	); err != nil {
		log.Printf("[personal-migration] set flag: %v", err)
	}

	log.Printf("[personal-migration] DONE — migrated %d global credential(s) to user id=%d", migrated, target)
}

// resolveMigrationTargetUser reads MIGRATION_TARGET_EMAIL from environment.
// If not set, migration is skipped — no hardcoded fallbacks.
func resolveMigrationTargetUser() int64 {
	email := os.Getenv("MIGRATION_TARGET_EMAIL")
	if email == "" {
		log.Println("[personal-migration] MIGRATION_TARGET_EMAIL not set; skipping migration")
		return 0
	}
	var id int64
	err := db.DB.QueryRow(
		`SELECT id FROM users WHERE LOWER(email) = LOWER(?) LIMIT 1`, email,
	).Scan(&id)
	if err == nil && id > 0 {
		log.Printf("[personal-migration] target: %s (id=%d)", email, id)
		return id
	}
	log.Printf("[personal-migration] MIGRATION_TARGET_EMAIL=%s not found in users table", email)
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
