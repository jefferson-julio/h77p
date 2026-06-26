package ui

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// Token colors for the .http file format.
var (
	clrSection   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	clrKeyword   = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))
	clrDelim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	clrHeaderKey = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	clrHeaderSep = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	clrHeaderVal = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	clrURL       = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	clrVar       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	clrStatus2xx = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	clrStatus3xx = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	clrStatus4xx = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	clrStatus5xx = lipgloss.NewStyle().Foreground(lipgloss.Color("197"))
)

// JS highlighting via chroma — used for {%...%} script block content.
var (
	jsLexer = lexers.Get("javascript")
	jsStyle = styles.Get("monokai")
	jsFmtr  = formatters.Get("terminal256")
)

var knownMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "HEAD": true, "OPTIONS": true, "TRACE": true, "CONNECT": true,
}

type hlState int

const (
	hlNormal    hlState = iota
	hlScript            // inside {%...%}
	hlExStatus          // inside @example {%, expecting HTTP/x.x status line
	hlExHeaders         // inside @example, reading response headers
	hlExBody            // inside @example, reading response body
)

// highlightHTTP processes our extended .http format line by line and returns
// an ANSI-coloured string suitable for display in a terminal viewport.
func highlightHTTP(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, len(lines))
	state := hlNormal
	for i, line := range lines {
		out[i], state = colorLine(line, state)
	}
	return strings.Join(out, "\n")
}

func colorLine(raw string, state hlState) (string, hlState) {
	trimmed := strings.TrimSpace(raw)

	switch state {
	case hlScript:
		if trimmed == "%}" {
			return clrDelim.Render(raw), hlNormal
		}
		return colorJS(raw), hlScript

	case hlExStatus:
		if trimmed == "%}" {
			return clrDelim.Render(raw), hlNormal
		}
		if strings.HasPrefix(trimmed, "HTTP/") {
			return colorStatusLine(raw), hlExHeaders
		}
		return raw, hlExStatus

	case hlExHeaders:
		if trimmed == "" {
			return raw, hlExBody
		}
		if trimmed == "%}" {
			return clrDelim.Render(raw), hlNormal
		}
		return colorHeader(raw), hlExHeaders

	case hlExBody:
		if trimmed == "%}" {
			return clrDelim.Render(raw), hlNormal
		}
		return raw, hlExBody
	}

	// hlNormal
	switch {
	case strings.HasPrefix(trimmed, "###"):
		return clrSection.Render(raw), hlNormal

	case trimmed == "@pre-request {%" || trimmed == "@post-response {%":
		kw := strings.TrimSpace(strings.TrimSuffix(trimmed, "{%"))
		return clrKeyword.Render(kw) + " " + clrDelim.Render("{%"), hlScript

	case trimmed == "@example {%":
		return clrKeyword.Render("@example") + " " + clrDelim.Render("{%"), hlExStatus

	case trimmed == "%}":
		return clrDelim.Render(raw), hlNormal

	case isMethodLine(trimmed):
		return colorMethodLine(raw), hlNormal

	case isHeaderLine(trimmed):
		return colorHeader(raw), hlNormal
	}

	return raw, hlNormal
}

func isMethodLine(line string) bool {
	m, _, ok := strings.Cut(line, " ")
	return ok && knownMethods[strings.ToUpper(m)]
}

// isHeaderLine avoids false-positives on JSON body lines that contain colons.
func isHeaderLine(line string) bool {
	if len(line) == 0 {
		return false
	}
	switch line[0] {
	case '{', '[', '"', ' ', '\t':
		return false
	}
	k, _, ok := strings.Cut(line, ":")
	return ok && !strings.ContainsAny(k, " \t\"'{}[]")
}

func colorMethodLine(raw string) string {
	method, rest, _ := strings.Cut(strings.TrimSpace(raw), " ")
	ms, ok := styleMethod[strings.ToUpper(method)]
	if !ok {
		ms = lipgloss.NewStyle()
	}
	return ms.Bold(true).Render(method) + " " + colorizeTokens(strings.TrimSpace(rest), clrURL)
}

func colorHeader(raw string) string {
	k, v, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok {
		return raw
	}
	return clrHeaderKey.Render(k) + clrHeaderSep.Render(":") + " " + colorizeTokens(strings.TrimSpace(v), clrHeaderVal)
}

func colorStatusLine(raw string) string {
	trimmed := strings.TrimSpace(raw)
	parts := strings.SplitN(trimmed, " ", 3)
	if len(parts) < 2 || len(parts[1]) == 0 {
		return raw
	}
	var clr lipgloss.Style
	switch parts[1][0] {
	case '2':
		clr = clrStatus2xx
	case '3':
		clr = clrStatus3xx
	case '4':
		clr = clrStatus4xx
	default:
		clr = clrStatus5xx
	}
	return clr.Render(trimmed)
}

// colorizeTokens wraps s in base style, but highlights {{var}} spans with clrVar.
func colorizeTokens(s string, base lipgloss.Style) string {
	if !strings.Contains(s, "{{") {
		return base.Render(s)
	}
	var b strings.Builder
	for {
		start := strings.Index(s, "{{")
		if start == -1 {
			b.WriteString(base.Render(s))
			break
		}
		if start > 0 {
			b.WriteString(base.Render(s[:start]))
		}
		end := strings.Index(s[start:], "}}")
		if end == -1 {
			b.WriteString(clrVar.Render(s[start:]))
			break
		}
		end += start + 2
		b.WriteString(clrVar.Render(s[start:end]))
		s = s[end:]
	}
	return b.String()
}

// colorJS runs a single line through chroma's JavaScript lexer.
// Falls back to a dim style if the lexer is unavailable.
func colorJS(raw string) string {
	if jsLexer == nil || jsFmtr == nil || jsStyle == nil {
		return styleDim.Render(raw)
	}
	iter, err := jsLexer.Tokenise(nil, raw)
	if err != nil {
		return styleDim.Render(raw)
	}
	var buf bytes.Buffer
	if err := jsFmtr.Format(&buf, jsStyle, iter); err != nil {
		return styleDim.Render(raw)
	}
	return strings.TrimRight(buf.String(), "\n")
}

