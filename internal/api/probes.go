package api

import (
	"crypto/tls"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/db"
)

// RunTLSProbe checks TLS certificate for a site
func RunTLSProbe(siteID int64, hostname string) {
	date := time.Now().Format("2006-01-02")
	hostname = strings.TrimPrefix(hostname, "https://")
	hostname = strings.TrimPrefix(hostname, "http://")
	hostname = strings.TrimRight(hostname, "/")
	if hostname == "" {
		return
	}

	// Strip path, keep host
	if idx := strings.Index(hostname, "/"); idx > 0 {
		hostname = hostname[:idx]
	}

	// Resolve hostname first to check for private IPs
	ips, err := net.LookupHost(hostname)
	if err != nil || len(ips) == 0 {
		db.DB.Exec(`INSERT INTO tls_checks_daily (site_id, date, hostname, cert_valid) VALUES ($1,$2,$3,FALSE)
			ON CONFLICT (site_id, date, hostname) DO UPDATE SET cert_valid=FALSE`,
			siteID, date, hostname)
		return
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
			log.Printf("[tls] site %d %s: blocked private IP %s", siteID, hostname, ipStr)
			return
		}
	}

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", hostname+":443", &tls.Config{})
	if err != nil {
		db.DB.Exec(`INSERT INTO tls_checks_daily (site_id, date, hostname, cert_valid) VALUES ($1,$2,$3,FALSE)
			ON CONFLICT (site_id, date, hostname) DO UPDATE SET cert_valid=FALSE`,
			siteID, date, hostname)
		return
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return
	}
	cert := state.PeerCertificates[0]
	daysUntilExpiry := int(time.Until(cert.NotAfter).Hours() / 24)
	tlsVersion := ""
	switch state.Version {
	case tls.VersionTLS13:
		tlsVersion = "TLS 1.3"
	case tls.VersionTLS12:
		tlsVersion = "TLS 1.2"
	case tls.VersionTLS11:
		tlsVersion = "TLS 1.1"
	default:
		tlsVersion = "TLS 1.0 or lower"
	}

	// Check HSTS
	hstsPresent := false
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Head("https://" + hostname)
	if err == nil {
		if resp.Header.Get("Strict-Transport-Security") != "" {
			hstsPresent = true
		}
		resp.Body.Close()
	}

	db.DB.Exec(`INSERT INTO tls_checks_daily (site_id, date, hostname, cert_valid, cert_issuer, valid_from, valid_to, days_until_expiry, tls_version, hsts_present)
		VALUES ($1,$2,$3,TRUE,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (site_id, date, hostname) DO UPDATE SET cert_valid=TRUE, cert_issuer=EXCLUDED.cert_issuer,
		valid_from=EXCLUDED.valid_from, valid_to=EXCLUDED.valid_to, days_until_expiry=EXCLUDED.days_until_expiry,
		tls_version=EXCLUDED.tls_version, hsts_present=EXCLUDED.hsts_present`,
		siteID, date, hostname, cert.Issuer.CommonName, cert.NotBefore, cert.NotAfter, daysUntilExpiry, tlsVersion, hstsPresent)

	log.Printf("[tls] site %d %s: valid, expires in %d days, %s", siteID, hostname, daysUntilExpiry, tlsVersion)
}

// RunDNSProbe checks DNS records for a site
func RunDNSProbe(siteID int64, hostname string) {
	date := time.Now().Format("2006-01-02")
	hostname = strings.TrimPrefix(hostname, "https://")
	hostname = strings.TrimPrefix(hostname, "http://")
	hostname = strings.TrimRight(hostname, "/")
	if idx := strings.Index(hostname, "/"); idx > 0 {
		hostname = hostname[:idx]
	}
	if hostname == "" {
		return
	}

	start := time.Now()
	ips, _ := net.LookupHost(hostname)
	resTime := time.Since(start).Milliseconds()

	nsList, _ := net.LookupNS(hostname)
	hasSPF := false
	hasDMARC := false
	txts, _ := net.LookupTXT(hostname)
	for _, t := range txts {
		if strings.HasPrefix(t, "v=spf1") {
			hasSPF = true
		}
	}
	dmarcTxts, _ := net.LookupTXT("_dmarc." + hostname)
	for _, t := range dmarcTxts {
		if strings.HasPrefix(t, "v=DMARC1") {
			hasDMARC = true
		}
	}

	aJSON, _ := json.Marshal(ips)
	if len(ips) == 0 { aJSON = []byte("[]") }

	nsHosts := make([]string, len(nsList))
	for i, ns := range nsList {
		nsHosts[i] = ns.Host
	}
	nsJSON, _ := json.Marshal(nsHosts)
	if len(nsList) == 0 { nsJSON = []byte("[]") }

	db.DB.Exec(`INSERT INTO dns_checks_daily (site_id, date, hostname, a_records_json, ns_json, resolution_time_ms, has_spf, has_dmarc)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (site_id, date, hostname) DO UPDATE SET a_records_json=EXCLUDED.a_records_json,
		ns_json=EXCLUDED.ns_json, resolution_time_ms=EXCLUDED.resolution_time_ms,
		has_spf=EXCLUDED.has_spf, has_dmarc=EXCLUDED.has_dmarc`,
		siteID, date, hostname, string(aJSON), string(nsJSON), resTime, hasSPF, hasDMARC)

	log.Printf("[dns] site %d %s: %d IPs, %dms, SPF=%v DMARC=%v", siteID, hostname, len(ips), resTime, hasSPF, hasDMARC)
}

// RunSecurityHeadersProbe checks security headers for a site
func RunSecurityHeadersProbe(siteID int64, rawURL string) {
	date := time.Now().Format("2006-01-02")
	if !strings.HasPrefix(rawURL, "http") {
		rawURL = "https://" + rawURL
	}

	client := crawler.BuildHTTPClient(15 * time.Second)
	resp, err := client.Head(rawURL)
	if err != nil {
		return
	}
	resp.Body.Close()

	hsts := resp.Header.Get("Strict-Transport-Security")
	csp := resp.Header.Get("Content-Security-Policy")
	xfo := resp.Header.Get("X-Frame-Options")
	rp := resp.Header.Get("Referrer-Policy")

	score := 0
	if hsts != "" {
		score += 25
	}
	if csp != "" {
		score += 25
	}
	if xfo != "" {
		score += 25
	}
	if rp != "" {
		score += 25
	}

	db.DB.Exec(`INSERT INTO security_headers_daily (site_id, date, url, hsts, csp, x_frame_options, referrer_policy, score)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (site_id, date, url) DO UPDATE SET hsts=EXCLUDED.hsts, csp=EXCLUDED.csp,
		x_frame_options=EXCLUDED.x_frame_options, referrer_policy=EXCLUDED.referrer_policy, score=EXCLUDED.score`,
		siteID, date, rawURL, hsts, csp, xfo, rp, score)
}

// RunAllProbes runs TLS, DNS, and security header probes for a site
func RunAllProbes(siteID int64, domain string) {
	RunTLSProbe(siteID, domain)
	RunDNSProbe(siteID, domain)
	RunSecurityHeadersProbe(siteID, domain)
}
