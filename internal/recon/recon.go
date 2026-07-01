package recon

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CertInfo holds TLS certificate details from a direct probe.
type CertInfo struct {
	CommonName  string   `json:"common_name,omitempty"`
	Issuer      string   `json:"issuer,omitempty"`
	ValidFrom   string   `json:"valid_from,omitempty"`
	ValidUntil  string   `json:"valid_until,omitempty"`
	SANs        []string `json:"sans,omitempty"`
	OriginHints []string `json:"origin_hints,omitempty"`
}

// TargetProfile holds auto-detected information about a target.
type TargetProfile struct {
	URL                string            `json:"url"`
	Host               string            `json:"host,omitempty"`
	FinalURL           string            `json:"final_url,omitempty"`
	StatusCode         int               `json:"status_code,omitempty"`
	WAF                string            `json:"waf"`
	CDNProviders       []string          `json:"cdn_providers,omitempty"`
	Server             string            `json:"server"`
	TechStack          []string          `json:"tech_stack"`
	CMS                string            `json:"cms,omitempty"`
	Frameworks         []string          `json:"frameworks,omitempty"`
	Analytics          []string          `json:"analytics,omitempty"`
	SecurityHeaders    map[string]string `json:"security_headers,omitempty"`
	MissingSecHeaders  []string          `json:"missing_sec_headers,omitempty"`
	NotableHeaders     map[string]string `json:"notable_headers,omitempty"`
	ResolvedIPs        []string          `json:"resolved_ips,omitempty"`
	CDNIPs             []string          `json:"cdn_ips,omitempty"`
	OriginCandidates   []string          `json:"origin_candidates,omitempty"`
	Cert               *CertInfo         `json:"cert,omitempty"`
	HasCF              bool              `json:"has_cloudflare"`
	HasAkamai          bool              `json:"has_akamai"`
	HasFastly          bool              `json:"has_fastly"`
	Redirects          []string          `json:"redirects,omitempty"`
	Notes              string            `json:"notes,omitempty"`
	RecommendedVectors []string          `json:"recommended_vectors"`
	RecommendedCombo   string            `json:"recommended_combo"`
	RecommendedL7Mode  string            `json:"recommended_l7_mode,omitempty"`
	AttackIntensity    string            `json:"attack_intensity,omitempty"`
}

var cdnIPPrefixes = []string{
	"104.", "103.21", "103.22", "103.31", "141.", "162.", "172.",
	"173.", "188.114", "190.", "197.234", "198.41", "131.0.72", "185.",
}

var secHeaderNames = []string{
	"Strict-Transport-Security",
	"Content-Security-Policy",
	"X-Frame-Options",
	"X-Content-Type-Options",
	"X-XSS-Protection",
	"Referrer-Policy",
	"Permissions-Policy",
}

// Analyze performs lightweight self-contained reconnaissance on a URL.
// No external paid APIs. Uses only stdlib + direct HTTP/TLS probes.
func Analyze(ctx context.Context, rawURL string) (*TargetProfile, error) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	profile := &TargetProfile{
		URL:              rawURL,
		Host:             parsed.Hostname(),
		TechStack:        []string{},
		Frameworks:       []string{},
		Analytics:        []string{},
		SecurityHeaders:  make(map[string]string),
		NotableHeaders:   make(map[string]string),
		RecommendedVectors: []string{},
	}

	resolveDNS(ctx, profile)

	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			},
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 5 * time.Second,
			}).DialContext,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SH-MVDoS-Recon/2.0 (Lab Research)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		if parsed.Scheme == "https" {
			parsed.Scheme = "http"
			req, _ = http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
			req.Header.Set("User-Agent", "SH-MVDoS-Recon/2.0 (Lab Research)")
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
			resp, err = client.Do(req)
		}
		if err != nil {
			return profile, fmt.Errorf("probe failed: %w", err)
		}
	}
	defer resp.Body.Close()

	profile.FinalURL = resp.Request.URL.String()
	profile.StatusCode = resp.StatusCode

	detectFromResponseHeaders(resp.Header, profile)

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	body := strings.ToLower(string(bodyBytes))
	detectFromBody(body, profile)

	if parsed.Scheme == "https" || strings.HasPrefix(profile.FinalURL, "https") {
		host := parsed.Hostname()
		if u, uerr := url.Parse(profile.FinalURL); uerr == nil && u.Hostname() != "" {
			host = u.Hostname()
		}
		collectCertInfo(host, profile)
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		if loc := resp.Header.Get("Location"); loc != "" {
			profile.Redirects = append(profile.Redirects, loc)
		}
	}

	classifyIPs(profile)
	applyRecommendations(profile)

	return profile, nil
}

func resolveDNS(ctx context.Context, p *TargetProfile) {
	if p.Host == "" {
		return
	}
	resolver := &net.Resolver{}
	ips, err := resolver.LookupHost(ctx, p.Host)
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip != "" && !seen[ip] {
			seen[ip] = true
			p.ResolvedIPs = append(p.ResolvedIPs, ip)
		}
	}
}

func detectFromResponseHeaders(hdr http.Header, p *TargetProfile) {
	providers := map[string]bool{}

	for k, v := range hdr {
		lower := strings.ToLower(k)
		val := strings.Join(v, ", ")

		if lower == "server" {
			p.Server = val
		}
		if strings.HasPrefix(lower, "x-powered-by") {
			p.TechStack = appendUnique(p.TechStack, val)
		}
		if lower == "set-cookie" {
			detectFromCookies(val, p)
		}

		switch {
		case lower == "cf-ray" || strings.Contains(val, "cloudflare") || strings.Contains(lower, "cf-"):
			p.HasCF = true
			providers["Cloudflare"] = true
			p.WAF = "Cloudflare"
		case strings.Contains(lower, "akamai") || strings.Contains(val, "akamai"):
			p.HasAkamai = true
			providers["Akamai"] = true
			if p.WAF == "" {
				p.WAF = "Akamai"
			}
		case strings.Contains(lower, "fastly") || strings.Contains(val, "fastly"):
			p.HasFastly = true
			providers["Fastly"] = true
			if p.WAF == "" {
				p.WAF = "Fastly"
			}
		case lower == "x-amz-cf-id" || lower == "x-amz-cf-pop":
			providers["AWS CloudFront"] = true
		case lower == "x-sucuri-id":
			providers["Sucuri WAF"] = true
			if p.WAF == "" {
				p.WAF = "Sucuri"
			}
		case lower == "x-cdn" && strings.Contains(strings.ToLower(val), "imperva"):
			providers["Imperva/Incapsula"] = true
			if p.WAF == "" {
				p.WAF = "Imperva"
			}
		case lower == "x-guploader-uploadid":
			providers["Google Cloud CDN"] = true
		}

		notable := []string{
			"cf-ray", "cf-cache-status", "x-amz-cf-id", "x-amz-cf-pop",
			"x-akamai-transformed", "x-fastly-request-id", "x-sucuri-id",
			"x-cdn", "via", "x-cache", "x-powered-by",
		}
		for _, n := range notable {
			if lower == n {
				p.NotableHeaders[k] = val
			}
		}
	}

	for _, h := range secHeaderNames {
		if v := hdr.Get(h); v != "" {
			p.SecurityHeaders[h] = v
		} else {
			p.MissingSecHeaders = append(p.MissingSecHeaders, h)
		}
	}

	if len(providers) == 0 && p.WAF == "" {
		p.CDNProviders = []string{"Unknown/Direct"}
	} else {
		for name := range providers {
			p.CDNProviders = appendUnique(p.CDNProviders, name)
		}
	}
}

func classifyIPs(p *TargetProfile) {
	cdnSeen := map[string]bool{}
	originSeen := map[string]bool{}

	for _, ip := range p.ResolvedIPs {
		if isCDNIP(ip) {
			if !cdnSeen[ip] {
				cdnSeen[ip] = true
				p.CDNIPs = append(p.CDNIPs, ip)
			}
		} else if !originSeen[ip] {
			originSeen[ip] = true
			p.OriginCandidates = append(p.OriginCandidates, ip)
		}
	}

	if p.Cert != nil {
		for _, hint := range p.Cert.OriginHints {
			if !originSeen[hint] && !isCDNIP(hint) {
				originSeen[hint] = true
				p.OriginCandidates = append(p.OriginCandidates, hint)
			}
		}
	}
}

func isCDNIP(ip string) bool {
	for _, prefix := range cdnIPPrefixes {
		if strings.HasPrefix(ip, prefix) {
			return true
		}
	}
	return false
}

func appendUnique(list []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return list
	}
	for _, e := range list {
		if strings.EqualFold(e, item) {
			return list
		}
	}
	return append(list, item)
}

func detectFromCookies(cookieHeader string, p *TargetProfile) {
	c := strings.ToLower(cookieHeader)
	if strings.Contains(c, "phpsessid") {
		p.TechStack = appendUnique(p.TechStack, "PHP")
	}
	if strings.Contains(c, "asp.net_sessionid") || strings.Contains(c, "aspsession") {
		p.TechStack = appendUnique(p.TechStack, "ASP.NET")
	}
	if strings.Contains(c, "wordpress") || strings.Contains(c, "wp-") {
		p.CMS = "WordPress"
		p.TechStack = appendUnique(p.TechStack, "PHP")
	}
	if strings.Contains(c, "laravel") || strings.Contains(c, "laravel_session") {
		p.TechStack = appendUnique(p.TechStack, "Laravel/PHP")
	}
}

func detectFromBody(body string, p *TargetProfile) {
	cmsPatterns := map[string][]string{
		"WordPress":   {"wp-content", "wp-includes", "/wp-json/"},
		"Joomla":      {"joomla", "com_content"},
		"Drupal":      {"drupal", "/sites/default/"},
		"Magento":     {"magento", "mage/cookies", "catalogsearch"},
		"Shopify":     {"shopify", "cdn.shopify.com", "myshopify.com"},
		"WooCommerce": {"woocommerce", "wc-ajax", "add-to-cart"},
		"PrestaShop":  {"prestashop", "presta"},
		"OpenCart":    {"opencart", "route=product"},
		"Wix":         {"wix.com", "wixstatic"},
		"Squarespace": {"squarespace"},
		"Ghost":       {"ghost"},
		"Webflow":     {"webflow"},
	}
	for cms, patterns := range cmsPatterns {
		for _, pat := range patterns {
			if strings.Contains(body, pat) {
				p.CMS = cms
				break
			}
		}
	}

	fwPatterns := map[string][]string{
		"React":     {"react", "data-reactroot"},
		"Vue.js":    {"vue", "data-v-"},
		"Next.js":   {"next.js", "__next"},
		"Nuxt.js":   {"__nuxt", "_nuxt/"},
		"Angular":   {"angular", "ng-version"},
		"jQuery":    {"jquery"},
		"Alpine.js": {"x-data", "alpine"},
		"HTMX":      {"htmx.org", "hx-"},
	}
	for fw, patterns := range fwPatterns {
		for _, pat := range patterns {
			if strings.Contains(body, pat) {
				p.Frameworks = appendUnique(p.Frameworks, fw)
				break
			}
		}
	}

	analyticsPatterns := map[string][]string{
		"Google Analytics":   {"google-analytics", "gtag", "ga.js"},
		"Google Tag Manager": {"googletagmanager"},
		"Facebook Pixel":     {"facebook.com/tr", "fbq("},
		"Hotjar":             {"hotjar"},
		"Plausible":          {"plausible.io"},
	}
	for name, patterns := range analyticsPatterns {
		for _, pat := range patterns {
			if strings.Contains(body, pat) {
				p.Analytics = appendUnique(p.Analytics, name)
				break
			}
		}
	}

	if strings.Contains(body, "php") {
		p.TechStack = appendUnique(p.TechStack, "PHP")
	}
	if strings.Contains(body, "node") || strings.Contains(body, "express") {
		p.TechStack = appendUnique(p.TechStack, "Node.js")
	}
	if strings.Contains(body, "bootstrap") {
		p.TechStack = appendUnique(p.TechStack, "Bootstrap")
	}
	if strings.Contains(body, "tailwind") {
		p.TechStack = appendUnique(p.TechStack, "Tailwind CSS")
	}

	if p.CMS == "WordPress" {
		p.TechStack = appendUnique(p.TechStack, "PHP")
	}
	if p.CMS != "" && len(p.TechStack) == 0 {
		p.TechStack = append(p.TechStack, "PHP")
	}
}

func collectCertInfo(host string, p *TargetProfile) {
	if host == "" {
		return
	}
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", host+":443", &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return
	}
	cert := certs[0]

	info := &CertInfo{
		CommonName: cert.Subject.CommonName,
		ValidFrom:  cert.NotBefore.UTC().Format(time.RFC3339),
		ValidUntil: cert.NotAfter.UTC().Format(time.RFC3339),
	}
	if issuer := cert.Issuer.CommonName; issuer != "" {
		info.Issuer = issuer
		if strings.Contains(strings.ToLower(issuer), "cloudflare") {
			p.HasCF = true
			p.WAF = "Cloudflare"
			p.CDNProviders = appendUnique(p.CDNProviders, "Cloudflare")
		}
	}

	limit := 8
	for _, san := range cert.DNSNames {
		if limit <= 0 {
			break
		}
		info.SANs = append(info.SANs, san)
		limit--
		sanLower := strings.ToLower(san)
		if !strings.Contains(sanLower, "cloudflare") &&
			!strings.HasPrefix(san, "*.") &&
			san != host &&
			!strings.Contains(sanLower, "letsencrypt") {
			info.OriginHints = appendUnique(info.OriginHints, san)
		}
	}

	p.Cert = info
	if len(info.OriginHints) > 0 {
		p.Notes = appendNote(p.Notes, "Cert SAN hints: "+strings.Join(info.OriginHints, ", "))
	} else if info.Issuer != "" && p.Notes == "" {
		p.Notes = "Cert issuer: " + info.Issuer
	}
}

func applyRecommendations(p *TargetProfile) {
	vectors := []string{}
	combo := "baseline"
	l7Mode := "baseline"
	intensity := "medium"

	switch {
	case p.HasCF:
		vectors = append(vectors, "h2-rapid-reset", "l7-post", "l7-apiflood")
		combo = "protocol-storm"
		l7Mode = "httppost"
		intensity = "high"
		p.Notes = appendNote(p.Notes, "Cloudflare detected — HTTP/2 rapid reset and protocol storm recommended. Consider origin bypass if real IP available.")
	case p.HasAkamai:
		vectors = append(vectors, "h2-rapid-reset", "l7-post")
		combo = "protocol-storm"
		l7Mode = "httppost"
		intensity = "high"
	case p.CMS == "WordPress":
		vectors = append(vectors, "l7-wordpress", "wp-search", "l7-post", "slowloris")
		combo = "wordpress-abuse"
		l7Mode = "wordpress"
		p.Notes = appendNote(p.Notes, "WordPress detected — heartbeat + wp-search stress recommended.")
	case p.CMS == "WooCommerce":
		vectors = append(vectors, "woocommerce-search", "l7-wordpress", "wp-search", "l7-post")
		combo = "wordpress-abuse"
		l7Mode = "woocommerce-search"
		p.Notes = appendNote(p.Notes, "WooCommerce detected — product search + cart API stress.")
	case p.CMS == "Magento":
		vectors = append(vectors, "l7-magento-search", "l7-magento-cart", "l7-post", "slowloris")
		combo = "magento-abuse"
		l7Mode = "catalog-search"
		p.Notes = appendNote(p.Notes, "Magento detected — catalog-search + guest-cart REST stress.")
	case p.CMS == "Shopify":
		vectors = append(vectors, "l7-shopify-search", "l7-apiflood", "ws-flood")
		combo = "shopify-abuse"
		l7Mode = "shopify-search"
		p.Notes = appendNote(p.Notes, "Shopify detected — search suggest + cart/add.js stress.")
	case p.CMS == "Drupal":
		vectors = append(vectors, "drupal-search", "l7-post", "slowloris")
		combo = "cms-abuse"
		l7Mode = "drupal-search"
		p.Notes = appendNote(p.Notes, "Drupal detected — /search/node keys stress.")
	case p.CMS == "Joomla":
		vectors = append(vectors, "joomla-search", "l7-post", "slowloris")
		combo = "cms-abuse"
		l7Mode = "joomla-search"
		p.Notes = appendNote(p.Notes, "Joomla detected — com_search stress.")
	case p.CMS == "PrestaShop" || p.CMS == "OpenCart":
		vectors = append(vectors, "cms-rotate", "l7-post")
		combo = "cms-abuse"
		l7Mode = "cms-rotate"
		p.Notes = appendNote(p.Notes, p.CMS+" detected — catalog search stress.")
	case hasFramework(p.Frameworks, "Next.js"):
		vectors = append(vectors, "next-image", "l7-apiflood", "h2-rapid-reset")
		combo = "api-abuse"
		l7Mode = "next-image"
		p.Notes = appendNote(p.Notes, "Next.js detected — /_next/image optimizer stress.")
	case hasFramework(p.Frameworks, "React") || hasFramework(p.Frameworks, "Vue.js"):
		vectors = append(vectors, "l7-apiflood", "l7-graphql", "ws-flood", "h2-rapid-reset")
		combo = "api-abuse"
		l7Mode = "apiflood"
		p.Notes = appendNote(p.Notes, "SPA/API stack — JSON and GraphQL abuse recommended.")
	case strings.Contains(strings.ToLower(strings.Join(p.TechStack, " ")), "php"):
		vectors = append(vectors, "l7-post", "l7-rudy")
		combo = "slayer-mix"
		l7Mode = "httppost"
	default:
		vectors = append(vectors, "l7-baseline", "slowloris", "l7-rudy", "h2-rapid-reset")
		combo = "slayer-mix"
		l7Mode = "baseline"
	}

	if !contains(vectors, "slowloris") {
		vectors = append(vectors, "slowloris")
	}
	if !contains(vectors, "h2-rapid-reset") && (p.HasCF || p.HasAkamai || p.WAF != "") {
		vectors = append(vectors, "h2-rapid-reset")
	}
	if p.WAF == "" {
		vectors = append(vectors, "quic-burner")
	}

	p.RecommendedVectors = uniqueStrings(vectors)
	p.RecommendedCombo = combo
	p.RecommendedL7Mode = l7Mode
	p.AttackIntensity = intensity

	if p.WAF != "" && p.Notes == "" {
		p.Notes = fmt.Sprintf("%s detected — protocol-aware vectors advised.", p.WAF)
	}
	if len(p.OriginCandidates) > 0 {
		p.Notes = appendNote(p.Notes, fmt.Sprintf("Origin candidates: %s", strings.Join(p.OriginCandidates, ", ")))
	}
}

func appendNote(existing, note string) string {
	if existing == "" {
		return note
	}
	return existing + " | " + note
}

func hasFramework(frameworks []string, name string) bool {
	for _, f := range frameworks {
		if strings.EqualFold(f, name) {
			return true
		}
	}
	return false
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// NormalizeTarget is a helper for UI.
func NormalizeTarget(raw string) string {
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "http") {
		return "https://" + raw
	}
	return raw
}