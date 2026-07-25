package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// @path / @./file / @folder mentions (Cursor/Grok-style attach).
var reMention = regexp.MustCompile(`@([^\s,;:]+)`)

// expandMentions injects file/dir contents for @path tokens in the user message.
// Returns augmented text and a short human summary of attachments.
func expandMentions(user string) (string, string) {
	matches := reMention.FindAllStringSubmatch(user, -1)
	if len(matches) == 0 {
		return user, ""
	}
	seen := map[string]struct{}{}
	var blocks []string
	var names []string
	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		raw = strings.Trim(raw, `"'`)
		if raw == "" || raw == "mentions" {
			continue
		}
		// skip emails-ish
		if strings.Contains(raw, "@") {
			continue
		}
		path := raw
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		block, label := loadMention(path)
		if block == "" {
			continue
		}
		blocks = append(blocks, block)
		names = append(names, label)
	}
	if len(blocks) == 0 {
		return user, ""
	}
	aug := user + "\n\n---\nAttached context from @mentions:\n" + strings.Join(blocks, "\n\n")
	return aug, strings.Join(names, ", ")
}

func loadMention(path string) (block string, label string) {
	// strip leading ./
	path = strings.TrimPrefix(path, "./")
	st, err := os.Stat(path)
	if err != nil {
		// try as relative from cwd already failed
		return fmt.Sprintf("[mention @%s: not found]", path), path
	}
	if st.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Sprintf("[mention @%s: %v]", path, err), path
		}
		var b strings.Builder
		fmt.Fprintf(&b, "### directory @%s\n", path)
		n := 0
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") && e.Name() != ".gitignore" {
				continue
			}
			suffix := ""
			if e.IsDir() {
				suffix = "/"
			}
			fmt.Fprintf(&b, "- %s%s\n", e.Name(), suffix)
			n++
			if n >= 80 {
				b.WriteString("…\n")
				break
			}
		}
		return b.String(), path + "/"
	}
	// file
	if st.Size() > 100_000 {
		return fmt.Sprintf("[mention @%s: file too large (%d bytes); use read_file]", path, st.Size()), path
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("[mention @%s: %v]", path, err), path
	}
	content := string(data)
	const max = 24_000
	if len(content) > max {
		content = content[:max] + "\n…[truncated]…"
	}
	ext := filepath.Ext(path)
	lang := strings.TrimPrefix(ext, ".")
	return fmt.Sprintf("### file @%s\n```%s\n%s\n```", path, lang, content), path
}
