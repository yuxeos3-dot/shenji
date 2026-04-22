package api

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"seo-system/internal/db"
)

// FlattenAuditFindings parses a v6_report JSON blob from audit_results
// and inserts each issue as a row in audit_findings.
// Called after every audit completes + one-time backfill for history.
func FlattenAuditFindings(taskID int64, siteID int64, v6JSON string) int {
	if v6JSON == "" || v6JSON == "{}" {
		return 0
	}

	var report struct {
		Chapters []struct {
			Title  string `json:"title"`
			Module string `json:"module"`
			Issues []struct {
				ID             string `json:"id"`
				Title          string `json:"title"`
				Severity       string `json:"severity"`
				Confidence     string `json:"confidence"`
				PageType       string `json:"page_type"`
				PageURL        string `json:"page_url"`
				Element        string `json:"element"`
				Evidence       string `json:"evidence"`
				EvidenceJSON   json.RawMessage `json:"evidence_json"`
				FixSuggestion  string `json:"fix_suggestion"`
				FixType        string `json:"fix_type"`
				Effort         string `json:"effort"`
			} `json:"issues"`
		} `json:"chapters"`
	}

	if err := json.Unmarshal([]byte(v6JSON), &report); err != nil {
		// Try alternative format: direct array of issues
		var altReport struct {
			Chapters []struct {
				ChapterTitle string `json:"chapter_title"`
				Issues       []struct {
					Title    string `json:"title"`
					Severity string `json:"severity"`
					Scope    string `json:"scope"`
					Evidence string `json:"evidence"`
					Fix      string `json:"fix"`
				} `json:"issues"`
			} `json:"chapters"`
		}
		if json.Unmarshal([]byte(v6JSON), &altReport) != nil {
			log.Printf("[flatten] failed to parse v6_report for task %d: first 200 chars: %.200s", taskID, v6JSON)
			return 0
		}
		// Convert alt format
		for _, ch := range altReport.Chapters {
			for _, iss := range ch.Issues {
				insertFinding(taskID, siteID, ch.ChapterTitle, ch.ChapterTitle, "",
					iss.Title, iss.Severity, "high", "", "", "",
					nil, iss.Fix, "", iss.Evidence)
			}
		}
		markFlattened(taskID)
		return len(altReport.Chapters)
	}

	count := 0
	for _, ch := range report.Chapters {
		module := ch.Module
		if module == "" {
			module = ch.Title
		}
		for _, iss := range ch.Issues {
			evidenceJSON := iss.EvidenceJSON
			if evidenceJSON == nil && iss.Evidence != "" {
				evidenceJSON, _ = json.Marshal(map[string]string{"text": iss.Evidence})
			}
			insertFinding(taskID, siteID, ch.Title, module, iss.ID,
				iss.Title, iss.Severity, iss.Confidence,
				iss.PageType, iss.PageURL, iss.Element,
				evidenceJSON, iss.FixSuggestion, iss.FixType, iss.Effort)
			count++
		}
	}

	markFlattened(taskID)
	log.Printf("[flatten] task %d site %d: %d findings extracted", taskID, siteID, count)
	return count
}

func insertFinding(taskID, siteID int64, chapter, module, issueCode,
	title, severity, confidence, pageType, pageURL, element string,
	evidenceJSON json.RawMessage, fixSuggestion, fixType, effort string) {

	if title == "" {
		return
	}
	if severity == "" {
		severity = "中等"
	}
	if confidence == "" {
		confidence = "high"
	}
	if evidenceJSON == nil {
		evidenceJSON = json.RawMessage("{}")
	}

	// Dedup: check if same issue seen before
	dedupKey := computeDedupKey(siteID, module, pageURL, title)
	var existingDedupID int64
	var occurrences int
	var firstTaskID int64
	err := db.DB.QueryRow(`SELECT id, occurrences, first_task_id FROM audit_finding_dedup
		WHERE site_id=$1 AND dedup_key=$2`, siteID, dedupKey).Scan(&existingDedupID, &occurrences, &firstTaskID)

	firstSeen := firstTaskID
	recurrence := 1
	if err == nil {
		// Existing — increment recurrence
		recurrence = occurrences + 1
		firstSeen = firstTaskID
		db.DB.Exec(`UPDATE audit_finding_dedup SET last_task_id=$1, last_seen_at=NOW(), occurrences=$2, latest_finding_id=0
			WHERE id=$3`, taskID, recurrence, existingDedupID)
	} else {
		// New dedup entry
		firstSeen = taskID
		db.DB.Exec(`INSERT INTO audit_finding_dedup (site_id, dedup_key, first_task_id, last_task_id, first_seen_at, last_seen_at, occurrences)
			VALUES ($1,$2,$3,$3,NOW(),NOW(),1) ON CONFLICT (site_id, dedup_key) DO NOTHING`,
			siteID, dedupKey, taskID)
	}

	var findingID int64
	db.DB.QueryRow(`INSERT INTO audit_findings (task_id, site_id, chapter, module, issue_code, title, severity,
		confidence, page_type, page_url, element_selector, evidence_json, fix_suggestion, fix_type,
		estimated_effort, first_seen_task_id, recurrence_count)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id`,
		taskID, siteID, chapter, module, issueCode, title, severity,
		confidence, pageType, pageURL, element, string(evidenceJSON),
		fixSuggestion, fixType, effort, firstSeen, recurrence).Scan(&findingID)

	// Update dedup with latest finding ID
	if findingID > 0 && existingDedupID > 0 {
		db.DB.Exec(`UPDATE audit_finding_dedup SET latest_finding_id=$1 WHERE id=$2`, findingID, existingDedupID)
	}
}

func computeDedupKey(siteID int64, module, pageURL, title string) string {
	// Normalize: lowercase, trim, remove numbers that might change between runs
	norm := strings.ToLower(strings.TrimSpace(module + "|" + pageURL + "|" + title))
	hash := md5.Sum([]byte(norm))
	return fmt.Sprintf("%x", hash)
}

func markFlattened(taskID int64) {
	db.DB.Exec(`UPDATE audit_results SET findings_flattened=TRUE WHERE task_id=$1`, taskID)
}

// BackfillAuditFindings runs once to flatten all historical audit_results
// that haven't been flattened yet.
func BackfillAuditFindings() {
	var flag string
	db.DB.QueryRow("SELECT value FROM settings WHERE key='audit_findings_backfilled'").Scan(&flag)
	if flag == "done" {
		return
	}

	rows, err := db.DB.Query(`SELECT ar.task_id, at.site_id, ar.v6_report
		FROM audit_results ar
		JOIN audit_tasks at ON ar.task_id = at.id
		WHERE (ar.findings_flattened IS NULL OR ar.findings_flattened = FALSE)
		  AND ar.v6_report != '' AND ar.v6_report != '{}'
		ORDER BY ar.task_id`)
	if err != nil {
		log.Printf("[backfill] query failed: %v", err)
		return
	}
	defer rows.Close()

	total := 0
	for rows.Next() {
		var taskID int64
		var siteID int64
		var v6JSON string
		if rows.Scan(&taskID, &siteID, &v6JSON) != nil {
			continue
		}
		n := FlattenAuditFindings(taskID, siteID, v6JSON)
		total += n
	}

	db.DB.Exec("INSERT INTO settings (key,value) VALUES ('audit_findings_backfilled','done') ON CONFLICT (key) DO UPDATE SET value='done'")
	log.Printf("[backfill] audit_findings: processed, total findings extracted: %d", total)
}

// UpdateAuditResultsCounts updates the summary count columns on audit_results
func UpdateAuditResultsCounts(taskID int64) {
	db.DB.Exec(`UPDATE audit_results SET
		critical_count = (SELECT COUNT(*) FROM audit_findings WHERE task_id=$1 AND severity IN ('致命','critical')),
		serious_count = (SELECT COUNT(*) FROM audit_findings WHERE task_id=$1 AND severity IN ('严重','serious','high')),
		medium_count = (SELECT COUNT(*) FROM audit_findings WHERE task_id=$1 AND severity IN ('中等','medium')),
		low_count = (SELECT COUNT(*) FROM audit_findings WHERE task_id=$1 AND severity IN ('低','low','info')),
		total_findings = (SELECT COUNT(*) FROM audit_findings WHERE task_id=$1)
		WHERE task_id=$1`, taskID)
}
