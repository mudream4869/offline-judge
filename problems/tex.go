package problems

import (
	"regexp"
	"strings"
)

// texToMarkdown converts the LaTeX subset of Kattis statements (problemtools'
// problem.cls) to Markdown: sections, emphasis, lists, simple tables and
// math, which stays as $…$ / $$…$$. Unknown commands are left as they are.
// Images aren't downloaded; they show as their caption or file name.
func texToMarkdown(tex string) string {
	s := stripTexComments(tex)
	// Before \problemname is setup (macros, lengths), not the statement.
	if i := indexCmd(s, "problemname"); i >= 0 {
		s = s[i:]
	}
	s = texVerb(s)
	s = strings.NewReplacer("\r\n", "\n", `\(`, "$", `\)`, "$", `\[`, "\n$$\n", `\]`, "\n$$\n").Replace(s)

	s = replaceCmd(s, "problemname", 1, func([]string) string { return "" })
	for _, c := range []struct {
		name, prefix string
	}{{"section*", "## "}, {"section", "## "}, {"subsection*", "### "}, {"subsection", "### "}} {
		prefix := c.prefix
		s = replaceCmd(s, c.name, 1, func(a []string) string { return "\n\n" + prefix + strings.TrimSpace(a[0]) + "\n\n" })
	}
	s = replaceCmd(s, "textbf", 1, func(a []string) string { return "**" + a[0] + "**" })
	for _, name := range []string{"emph", "textit"} {
		s = replaceCmd(s, name, 1, func(a []string) string { return "*" + a[0] + "*" })
	}
	s = replaceCmd(s, "texttt", 1, func(a []string) string { return "`" + a[0] + "`" })
	s = replaceCmd(s, "url", 1, func(a []string) string { return a[0] })
	s = replaceCmd(s, "href", 2, func(a []string) string { return "[" + a[1] + "](" + a[0] + ")" })
	s = replaceCmd(s, "includegraphics", 1, func(a []string) string { return "（圖：" + a[0] + "）" })
	// problemtools: \illustration{width}{file}{caption}
	s = replaceCmd(s, "illustration", 3, func(a []string) string {
		if c := strings.TrimSpace(a[2]); c != "" {
			return "\n\n（圖：" + c + "）\n\n"
		}
		return "\n\n（圖：" + a[1] + "）\n\n"
	})

	s = texEnvironments(s)
	s = strings.NewReplacer(
		`\noindent`, "", `\centering`, "", `\medskip`, "", `\bigskip`, "", `\smallskip`, "",
		"``", "“", "''", "”", `\ldots`, "…", `\dots`, "…",
		`\%`, "%", `\&`, "&", `\#`, "#", `\_`, "_", "~", " ",
	).Replace(s)
	// A line break outside tables.
	s = strings.ReplaceAll(s, `\\`, "  \n")

	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s) + "\n"
}

// texVerb turns \verb|…|, with any delimiter, into `…`.
func texVerb(s string) string {
	var b strings.Builder
	for {
		i := indexCmd(s, "verb")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		rest := strings.TrimPrefix(s[i+len(`\verb`):], "*")
		if rest == "" {
			b.WriteString(s)
			return b.String()
		}
		d := rest[:1]
		end := strings.Index(rest[1:], d)
		if end < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i] + "`" + rest[1:1+end] + "`")
		s = rest[2+end:]
	}
}

// stripTexComments drops % comments, keeping \%.
func stripTexComments(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		for j := 0; j < len(l); j++ {
			if l[j] == '\\' {
				j++
				continue
			}
			if l[j] == '%' {
				lines[i] = l[:j]
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

// replaceCmd replaces \name[opt]{a1}...{an} with f(args).
func replaceCmd(s, name string, n int, f func([]string) string) string {
	var b strings.Builder
	for {
		i := indexCmd(s, name)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		rest := s[i+len(name)+1:]
		rest = strings.TrimLeft(rest, " ")
		if strings.HasPrefix(rest, "[") { // optional argument, dropped
			if j := strings.IndexByte(rest, ']'); j >= 0 {
				rest = rest[j+1:]
			}
		}
		var args []string
		for range n {
			rest = strings.TrimLeft(rest, " \n")
			arg, after, ok := braceArg(rest)
			if !ok {
				break
			}
			args, rest = append(args, arg), after
		}
		if len(args) < n {
			// Not the form we know; keep it.
			b.WriteString(`\` + name)
			s = s[i+len(name)+1:]
			continue
		}
		b.WriteString(f(args))
		s = rest
	}
}

// indexCmd finds \name not followed by a letter (so \section isn't \sectionx).
func indexCmd(s, name string) int {
	off := 0
	for {
		i := strings.Index(s[off:], `\`+name)
		if i < 0 {
			return -1
		}
		i += off
		end := i + 1 + len(name)
		if end == len(s) || !isLetter(s[end]) || strings.HasSuffix(name, "*") {
			if !strings.HasSuffix(name, "*") && end < len(s) && s[end] == '*' {
				off = end // \section* is a different command
				continue
			}
			return i
		}
		off = end
	}
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// braceArg reads {…} with nested braces from the start of s.
func braceArg(s string) (arg, rest string, ok bool) {
	if !strings.HasPrefix(s, "{") {
		return "", s, false
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[1:i], s[i+1:], true
			}
		}
	}
	return "", s, false
}

var texEnv = regexp.MustCompile(`(?s)\\begin\{(itemize|enumerate|tabular|center|figure|table)\}(\{[^}]*\})?(.*?)\\end\{(itemize|enumerate|tabular|center|figure|table)\}`)

// texEnvironments converts lists and tables, and drops layout wrappers.
func texEnvironments(s string) string {
	for {
		m := texEnv.FindStringSubmatchIndex(s)
		if m == nil {
			return s
		}
		env, body := s[m[2]:m[3]], s[m[6]:m[7]]
		var out string
		switch env {
		case "itemize", "enumerate":
			mark := "- "
			if env == "enumerate" {
				mark = "1. "
			}
			var items []string
			for _, it := range strings.Split(body, `\item`)[1:] {
				items = append(items, mark+strings.Join(strings.Fields(it), " "))
			}
			out = "\n\n" + strings.Join(items, "\n") + "\n\n"
		case "tabular":
			out = "\n\n" + texTable(body) + "\n\n"
		default:
			out = "\n\n" + body + "\n\n"
		}
		s = s[:m[0]] + out + s[m[1]:]
	}
}

// texTable converts tabular rows to a Markdown table, the first row as header.
func texTable(body string) string {
	body = strings.ReplaceAll(body, `\hline`, "")
	var rows []string
	for _, r := range strings.Split(body, `\\`) {
		cells := strings.Split(r, "&")
		for i, c := range cells {
			cells[i] = strings.ReplaceAll(strings.Join(strings.Fields(c), " "), "|", `\|`)
		}
		if strings.TrimSpace(strings.Join(cells, "")) == "" {
			continue
		}
		rows = append(rows, "| "+strings.Join(cells, " | ")+" |")
		if len(rows) == 1 {
			rows = append(rows, "|"+strings.Repeat(" --- |", len(cells)))
		}
	}
	return strings.Join(rows, "\n")
}
