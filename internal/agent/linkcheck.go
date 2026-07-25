package agent

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// reHTTPURL finds http(s) URLs in text (docs, grep output, etc.).
var reHTTPURL = regexp.MustCompile(`https?://[^\s\[\]<>"'\\)]+`)

const maxLinkCheckURLs = 20

// runDeterministicLinkCheck greps the workspace for URLs, fetches public ones,
// and returns a human report. Localhost / private URLs are listed as skipped
// (docs often include ORDS/demo endpoints that are not running on this host).
func (a *Agent) runDeterministicLinkCheck(ctx context.Context, emit func(Event)) string {
	if a.Tools == nil {
		return "error: tools not available for link check"
	}
	known := toolNameSet(a.Tools)
	if _, ok := known["grep"]; !ok {
		return "error: grep tool not registered"
	}
	if _, ok := known["fetch"]; !ok {
		return "error: fetch tool not registered"
	}

	emit(Event{Kind: EventStatus, Text: "link-check: searching docs for URLs…"})
	grepArgsList := []string{
		`{"pattern":"https?://","path":".","glob":"*.md","max_results":80}`,
		`{"pattern":"https?://","path":".","glob":"*.{rst,txt,html,adoc,toml,yml,yaml}","max_results":40}`,
		`{"pattern":"https?://","path":".","max_results":40}`,
	}
	var grepOut strings.Builder
	for i, args := range grepArgsList {
		if err := ctx.Err(); err != nil {
			return "cancelled"
		}
		emit(Event{Kind: EventToolStart, Tool: "grep", Text: args})
		out, err := a.Tools.Run(ctx, "grep", args)
		if err != nil {
			out = fmt.Sprintf("error: %v\n%s", err, out)
		}
		emit(Event{Kind: EventToolEnd, Tool: "grep", ToolOut: out})
		if strings.TrimSpace(out) != "" && !strings.HasPrefix(strings.TrimSpace(out), "no matches") &&
			!strings.HasPrefix(strings.TrimSpace(out), "error:") {
			grepOut.WriteString(out)
			grepOut.WriteByte('\n')
			n := len(extractHTTPURLs(grepOut.String()))
			if n >= 5 && i == 0 {
				break
			}
			if n > 0 && i >= 1 {
				break
			}
		}
	}

	raw := grepOut.String()
	all := extractHTTPURLs(raw)
	if len(all) == 0 {
		return "Link check: no http(s) URLs found under the current workspace (cwd).\n" +
			"Tip: start agenterm from the docs/repo root, or @path a README."
	}

	var external, local, template []string
	for _, u := range all {
		if isTemplatePlaceholderURL(u) {
			template = append(template, u)
			continue
		}
		if isLocalOrPrivateURL(u) {
			local = append(local, u)
		} else {
			external = append(external, u)
		}
	}

	type result struct {
		url    string
		status string
		ok     bool
	}
	var results []result
	okN, badN := 0, 0

	toFetch := external
	capped := false
	if len(toFetch) > maxLinkCheckURLs {
		toFetch = toFetch[:maxLinkCheckURLs]
		capped = true
	}

	for i, u := range toFetch {
		if err := ctx.Err(); err != nil {
			return "cancelled"
		}
		emit(Event{Kind: EventStatus, Text: fmt.Sprintf("link-check: fetch %d/%d %s", i+1, len(toFetch), truncateStr(u, 48))})
		args := fmt.Sprintf(`{"url":%q,"timeout_sec":10}`, u)
		emit(Event{Kind: EventToolStart, Tool: "fetch", Text: args})
		out, err := a.Tools.Run(ctx, "fetch", args)
		if err != nil {
			out = fmt.Sprintf("error: %v", err)
		}
		emit(Event{Kind: EventToolEnd, Tool: "fetch", ToolOut: out})
		st, ok := parseFetchStatus(out)
		if ok {
			okN++
		} else {
			badN++
		}
		results = append(results, result{url: u, status: st, ok: ok})
	}

	// Plain-language report (easy to read in TUI).
	var b strings.Builder
	b.WriteString("Link check summary\n")
	b.WriteString("==================\n\n")
	fmt.Fprintf(&b, "Scanned workspace for http(s) links.\n")
	fmt.Fprintf(&b, "  Total unique URLs found:     %d\n", len(all))
	fmt.Fprintf(&b, "  External (public) URLs:      %d\n", len(external))
	fmt.Fprintf(&b, "  Local/private (skipped):     %d\n", len(local))
	fmt.Fprintf(&b, "  Placeholders like $VAR:      %d (skipped — not real hosts)\n", len(template))
	fmt.Fprintf(&b, "  Actually fetched this run:   %d", len(results))
	if capped {
		fmt.Fprintf(&b, "  [cap %d; %d external not fetched]", maxLinkCheckURLs, len(external)-maxLinkCheckURLs)
	}
	b.WriteString("\n\n")

	// Verdict first — what the user cares about.
	switch {
	case len(results) == 0 && len(local)+len(template) > 0:
		b.WriteString("Verdict: nothing public to probe (only local or template URLs).\n\n")
	case badN == 0 && okN > 0:
		fmt.Fprintf(&b, "Verdict: PASS — all %d checked public links responded OK.\n\n", okN)
	case badN > 0 && okN > 0:
		fmt.Fprintf(&b, "Verdict: PARTIAL — %d OK, %d failed among checked public links.\n\n", okN, badN)
	case badN > 0 && okN == 0:
		fmt.Fprintf(&b, "Verdict: FAIL — %d checked public link(s) failed.\n\n", badN)
	default:
		b.WriteString("Verdict: no fetches performed.\n\n")
	}

	if badN > 0 {
		b.WriteString("Failed public links (need a real host or doc fix)\n")
		b.WriteString("------------------------------------------------\n")
		for _, r := range results {
			if !r.ok {
				fmt.Fprintf(&b, "  FAIL  %s\n        %s\n", r.url, r.status)
			}
		}
		b.WriteByte('\n')
	}

	if okN > 0 {
		b.WriteString("Working public links\n")
		b.WriteString("--------------------\n")
		for _, r := range results {
			if r.ok {
				fmt.Fprintf(&b, "  OK    %s\n", r.url)
			}
		}
		b.WriteByte('\n')
	}

	if len(local) > 0 {
		b.WriteString("Skipped: local / private (docs examples; not your Ollama tunnel)\n")
		b.WriteString("------------------------------------------------------------------\n")
		b.WriteString("These only work if that service is running (ORDS, app, etc.).\n")
		show := local
		if len(show) > 12 {
			show = show[:12]
		}
		for _, u := range show {
			fmt.Fprintf(&b, "  skip  %s\n", u)
		}
		if len(local) > 12 {
			fmt.Fprintf(&b, "  … +%d more local\n", len(local)-12)
		}
		b.WriteByte('\n')
	}

	if len(template) > 0 {
		b.WriteString("Skipped: template placeholders (not real URLs)\n")
		b.WriteString("----------------------------------------------\n")
		show := template
		if len(show) > 8 {
			show = show[:8]
		}
		for _, u := range show {
			fmt.Fprintf(&b, "  skip  %s\n", u)
		}
		if len(template) > 8 {
			fmt.Fprintf(&b, "  … +%d more placeholders\n", len(template)-8)
		}
		b.WriteByte('\n')
	}

	if capped {
		fmt.Fprintf(&b, "Note: only the first %d public URLs were fetched (of %d). Re-run after fixing docs if needed.\n",
			maxLinkCheckURLs, len(external))
	}
	return strings.TrimSpace(b.String())
}

func extractHTTPURLs(text string) []string {
	matches := reHTTPURL.FindAllString(text, -1)
	seen := map[string]struct{}{}
	var out []string
	for _, m := range matches {
		m = strings.TrimRight(m, ".,;:)]}>\"'")
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		u, err := url.Parse(m)
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		key := u.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// isTemplatePlaceholderURL detects docs that use $ENV or ${VAR} in the host.
func isTemplatePlaceholderURL(raw string) bool {
	if strings.Contains(raw, "$") || strings.Contains(raw, "${") {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := u.Hostname()
	// e.g. example.com placeholders, your-cluster.local already handled as local
	if strings.Contains(h, "<") || strings.Contains(h, ">") {
		return true
	}
	if strings.Contains(strings.ToUpper(h), "YOUR_") || strings.Contains(h, "example.com") {
		// keep example.com as external (usually real); only angle-bracket hosts
		_ = h
	}
	return false
}

// isLocalOrPrivateURL is true for localhost / loopback / RFC1918 — docs examples.
func isLocalOrPrivateURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return true
	}
	h := strings.ToLower(host)
	if h == "localhost" || h == "127.0.0.1" || h == "0.0.0.0" || h == "::1" ||
		h == "host.docker.internal" || strings.HasSuffix(h, ".local") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// hostname — treat *.localhost as local
		if strings.HasSuffix(h, ".localhost") {
			return true
		}
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return true
	}
	return false
}

func parseFetchStatus(out string) (status string, ok bool) {
	out = strings.TrimSpace(out)
	if out == "" {
		return "empty response", false
	}
	if strings.HasPrefix(out, "HTTP ") {
		line := out
		if i := strings.IndexByte(out, '\n'); i >= 0 {
			line = out[:i]
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			code := parts[1]
			if code == "200" || code == "201" || code == "204" || code == "301" || code == "302" || code == "304" {
				return line, true
			}
			if len(code) == 3 && code[0] == '3' {
				return line, true
			}
			return line, false
		}
		return line, false
	}
	if strings.HasPrefix(out, "error:") {
		return truncateStr(out, 120), false
	}
	return truncateStr(out, 80), false
}

func truncateStr(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
