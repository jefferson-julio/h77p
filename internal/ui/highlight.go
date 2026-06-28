package ui

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// Token colours for the .http file format. Populated by initHighlightStyles().
var (
	clrSection    lipgloss.Style
	clrKeyword    lipgloss.Style
	clrDelim      lipgloss.Style
	clrHeaderKey  lipgloss.Style
	clrHeaderSep  lipgloss.Style
	clrHeaderVal  lipgloss.Style
	clrURL        lipgloss.Style
	clrVar        lipgloss.Style
	clrInlineExpr lipgloss.Style // ${{js expression}} tokens
	clrStatus2xx  lipgloss.Style
	clrStatus3xx  lipgloss.Style
	clrStatus4xx  lipgloss.Style
	clrStatus5xx  lipgloss.Style
)

// inlineExprBodyRe matches ${{expr}} tokens in body text (including chroma output).
var inlineExprBodyRe = regexp.MustCompile(`\$\{\{[^}]+\}\}`)

// Chroma lexers/style/formatter shared across JS, JSON, and XML highlighting.
// jsStyle is set by initHighlightStyles() from the active theme.
var (
	jsLexer   = lexers.Get("javascript")
	jsonLexer = lexers.Get("json")
	xmlLexer  = lexers.Get("xml")
	jsStyle   = styles.Get("monokai") // overridden by initHighlightStyles
	jsFmtr    = formatters.Get("terminal256")
)

func initHighlightStyles() {
	t := activeTheme
	clrSection    = lipgloss.NewStyle().Bold(true).Foreground(t.SynSection)
	clrKeyword    = lipgloss.NewStyle().Foreground(t.SynKeyword)
	clrDelim      = lipgloss.NewStyle().Foreground(t.FgFaint)
	clrHeaderKey  = lipgloss.NewStyle().Foreground(t.SynHeaderKey)
	clrHeaderSep  = lipgloss.NewStyle().Foreground(t.FgFaint)
	clrHeaderVal  = lipgloss.NewStyle().Foreground(t.SynHeaderVal)
	clrURL        = lipgloss.NewStyle().Foreground(t.SynHeaderVal)
	clrVar        = lipgloss.NewStyle().Bold(true).Foreground(t.SynVar)
	clrInlineExpr = lipgloss.NewStyle().Bold(true).Foreground(t.SynInlineExpr)
	clrStatus2xx  = lipgloss.NewStyle().Foreground(t.Status2xx)
	clrStatus3xx  = lipgloss.NewStyle().Foreground(t.Status3xx)
	clrStatus4xx  = lipgloss.NewStyle().Foreground(t.Status4xx)
	clrStatus5xx  = lipgloss.NewStyle().Foreground(t.Status5xx)

	if s := styles.Get(t.ChromaStyle); s != nil {
		jsStyle = s
	} else {
		jsStyle = styles.Get("monokai")
	}
}

var knownMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "HEAD": true, "OPTIONS": true, "TRACE": true, "CONNECT": true,
}

// ---------------------------------------------------------------------------
// State machine types
// ---------------------------------------------------------------------------

type hlMode int

const (
	modeNormal     hlMode = iota
	modeScript            // inside {%...%}
	modeExStatus          // inside @example {%, expecting HTTP/x.x status line
	modeExHeaders         // inside @example {%, reading response headers
	modeExBody            // inside @example {%, reading response body
	modeReqHeaders        // after METHOD URL line, reading request headers
	modeReqBody           // reading request body
)

type ctKind int

const (
	ctPlain ctKind = iota
	ctJSON
	ctXML
	ctForm
)

type hlState struct {
	mode  hlMode
	ctype ctKind // content type detected from headers; carried into body mode
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

// highlightHTTP processes our extended .http format and returns an ANSI-coloured
// string suitable for display in a terminal viewport.
//
// Body regions (modeExBody / modeReqBody) are buffered and passed to the chroma
// lexer as a single string so the lexer has full structural context. Per-line
// tokenisation produces wrong bracket/punctuation colours for JSON/XML.
func highlightHTTP(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	var st hlState
	var bodyBuf []string

	flushBody := func() {
		if len(bodyBuf) == 0 {
			return
		}
		// Preserve trailing blank lines: the chroma formatter strips trailing
		// newlines, so we detach them and re-append after highlighting.
		end := len(bodyBuf)
		for end > 0 && strings.TrimSpace(bodyBuf[end-1]) == "" {
			end--
		}
		trailing := bodyBuf[end:]
		if end > 0 {
			body := strings.Join(bodyBuf[:end], "\n")
			var highlighted string
			switch st.ctype {
			case ctJSON:
				highlighted = colorJSON(body)
			case ctXML:
				highlighted = colorXML(body)
			case ctForm:
				highlighted = colorFormBody(body)
			default:
				highlighted = body
			}
			// Re-apply inline expression highlighting after chroma — chroma doesn't
			// know about ${{expr}} so these tokens appear verbatim in its output.
			highlighted = colorizeInlineExprs(highlighted)
			out = append(out, strings.Split(highlighted, "\n")...)
		}
		out = append(out, trailing...)
		bodyBuf = bodyBuf[:0]
	}

	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		switch st.mode {
		case modeExBody:
			if trimmed == "%}" {
				flushBody()
				out = append(out, clrDelim.Render(raw))
				st = hlState{}
			} else {
				bodyBuf = append(bodyBuf, raw)
			}
		case modeReqBody:
			if isBlockTag(trimmed) || strings.HasPrefix(trimmed, "###") || strings.HasPrefix(trimmed, "@jq ") {
				flushBody()
				colored, next := colorLineNormal(raw, trimmed)
				out = append(out, colored)
				st = next
			} else {
				bodyBuf = append(bodyBuf, raw)
			}
		default:
			colored, next := colorLine(raw, st)
			out = append(out, colored)
			st = next
		}
	}
	flushBody() // request body ending at EOF

	// Expand tabs to spaces so ansi.StringWidth (which counts \t as 1) matches
	// what the terminal actually renders (tab stops at every 8 columns).
	for i, line := range out {
		if strings.ContainsRune(line, '\t') {
			out[i] = strings.ReplaceAll(line, "\t", "    ")
		}
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------------------
// Line colouriser
// ---------------------------------------------------------------------------

func colorLine(raw string, state hlState) (string, hlState) {
	trimmed := strings.TrimSpace(raw)

	switch state.mode {
	case modeScript:
		if trimmed == "%}" {
			return clrDelim.Render(raw), hlState{}
		}
		return colorJS(raw), state

	case modeExStatus:
		if trimmed == "%}" {
			return clrDelim.Render(raw), hlState{}
		}
		if strings.HasPrefix(trimmed, "HTTP/") {
			return colorStatusLine(raw), hlState{mode: modeExHeaders}
		}
		return raw, state

	case modeExHeaders:
		if trimmed == "%}" {
			return clrDelim.Render(raw), hlState{}
		}
		if trimmed == "" {
			// Blank line: transition into body, carrying detected content type.
			return raw, hlState{mode: modeExBody, ctype: state.ctype}
		}
		next := state
		if ct, ok := parseContentType(trimmed); ok {
			next.ctype = ct
		}
		return colorHeader(raw), next

	case modeReqHeaders:
		// Block tags, ### separators, and @jq lines end request-header context.
		if isBlockTag(trimmed) || strings.HasPrefix(trimmed, "###") || strings.HasPrefix(trimmed, "@jq ") {
			return colorLineNormal(raw, trimmed)
		}
		if strings.HasPrefix(trimmed, "#") {
			return styleDim.Render(raw), state // comment inside headers — stay in header mode
		}
		if trimmed == "" {
			// Blank separator: transition into request body.
			return raw, hlState{mode: modeReqBody, ctype: state.ctype}
		}
		next := state
		if ct, ok := parseContentType(trimmed); ok {
			next.ctype = ct
		}
		return colorHeader(raw), next

	}

	// modeNormal
	return colorLineNormal(raw, trimmed)
}

// colorLineNormal handles lines in the top-level (non-body, non-script) context.
// It is also called when modeReqHeaders/modeReqBody see a block tag or ### line
// and need to re-enter normal-mode processing.
func colorLineNormal(raw, trimmed string) (string, hlState) {
	switch {
	case strings.HasPrefix(trimmed, "###"):
		return clrSection.Render(raw), hlState{}

	case strings.HasPrefix(trimmed, "#"):
		return styleDim.Render(raw), hlState{}

	case trimmed == "@pre-request {%" || trimmed == "@post-response {%":
		kw := strings.TrimSpace(strings.TrimSuffix(trimmed, "{%"))
		return clrKeyword.Render(kw) + " " + clrDelim.Render("{%"), hlState{mode: modeScript}

	case trimmed == "@example {%":
		return clrKeyword.Render("@example") + " " + clrDelim.Render("{%"), hlState{mode: modeExStatus}

	case trimmed == "%}":
		return clrDelim.Render(raw), hlState{}

	case strings.HasPrefix(trimmed, "@jq "):
		filter := strings.TrimSpace(strings.TrimPrefix(trimmed, "@jq "))
		return clrKeyword.Render("@jq") + " " + colorizeTokens(filter, clrURL), hlState{}

	case strings.HasPrefix(trimmed, "@") && !isBlockTag(trimmed) && strings.Contains(trimmed, "="):
		// Variable declaration: @name = value (file-level or request-level)
		rest := strings.TrimPrefix(trimmed, "@")
		name, val, ok := strings.Cut(rest, "=")
		if !ok {
			return raw, hlState{}
		}
		return clrKeyword.Render("@"+strings.TrimSpace(name)) + " " + clrHeaderSep.Render("=") + " " + colorizeTokens(strings.TrimSpace(val), clrHeaderVal), hlState{}

	case isMethodLine(trimmed):
		// After the method line we expect request headers next.
		return colorMethodLine(raw), hlState{mode: modeReqHeaders}

	case isHeaderLine(trimmed):
		return colorHeader(raw), hlState{}
	}

	return raw, hlState{}
}

// ---------------------------------------------------------------------------
// Body highlighting
// ---------------------------------------------------------------------------

// colorFormBody highlights a multi-line x-www-form-urlencoded body where each
// line is a "key=value" or "key=value&" pair.
func colorFormBody(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		amp := strings.HasSuffix(trimmed, "&")
		kv := trimmed
		if amp {
			kv = kv[:len(kv)-1]
		}
		key, val, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		rendered := clrHeaderKey.Render(key) + clrHeaderSep.Render("=") + colorizeTokens(val, clrHeaderVal)
		if amp {
			rendered += clrDelim.Render("&")
		}
		lines[i] = rendered
	}
	return strings.Join(lines, "\n")
}

// highlightBodyFromHeaders highlights a full body string (potentially multi-line)
// using the Content-Type header from the given map. Used by the live response panel.
func highlightBodyFromHeaders(body string, headers map[string][]string) string {
	if body == "" {
		return body
	}
	ctype := ctPlain
	for k, vs := range headers {
		if strings.EqualFold(k, "content-type") && len(vs) > 0 {
			v := strings.ToLower(vs[0])
			switch {
			case strings.Contains(v, "json"):
				ctype = ctJSON
			case strings.Contains(v, "xml"):
				ctype = ctXML
			case strings.Contains(v, "x-www-form-urlencoded"):
				ctype = ctForm
			}
			break
		}
	}
	if ctype == ctPlain {
		return body
	}
	switch ctype {
	case ctJSON:
		return colorJSON(body)
	case ctXML:
		return colorXML(body)
	case ctForm:
		return colorFormBody(body)
	}
	return body
}

// parseContentType parses a "Name: Value" header line. Returns (kind, true) only
// when the header name is Content-Type.
func parseContentType(headerLine string) (ctKind, bool) {
	name, value, ok := strings.Cut(strings.TrimSpace(headerLine), ":")
	if !ok || !strings.EqualFold(strings.TrimSpace(name), "content-type") {
		return ctPlain, false
	}
	v := strings.ToLower(value)
	switch {
	case strings.Contains(v, "json"):
		return ctJSON, true
	case strings.Contains(v, "xml"):
		return ctXML, true
	case strings.Contains(v, "x-www-form-urlencoded"):
		return ctForm, true
	default:
		return ctPlain, true
	}
}

// ---------------------------------------------------------------------------
// Chroma helpers
// ---------------------------------------------------------------------------

func colorJSON(raw string) string {
	if jsonLexer == nil || jsFmtr == nil || jsStyle == nil {
		return raw
	}
	iter, err := jsonLexer.Tokenise(nil, raw)
	if err != nil {
		return raw
	}
	var buf bytes.Buffer
	if err := jsFmtr.Format(&buf, jsStyle, iter); err != nil {
		return raw
	}
	return strings.TrimRight(buf.String(), "\n")
}

func colorXML(raw string) string {
	if xmlLexer == nil || jsFmtr == nil || jsStyle == nil {
		return raw
	}
	iter, err := xmlLexer.Tokenise(nil, raw)
	if err != nil {
		return raw
	}
	var buf bytes.Buffer
	if err := jsFmtr.Format(&buf, jsStyle, iter); err != nil {
		return raw
	}
	return strings.TrimRight(buf.String(), "\n")
}

// isBlockTag returns true for the block-opening tags that can appear within a
// request definition (@pre-request, @post-response, @example).
func isBlockTag(line string) bool {
	return line == "@pre-request {%" ||
		line == "@post-response {%" ||
		line == "@example {%"
}

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

// ---------------------------------------------------------------------------
// Line-level helpers (unchanged)
// ---------------------------------------------------------------------------

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
	rest = strings.TrimSpace(rest)
	ms, ok := styleMethod[strings.ToUpper(method)]
	if !ok {
		ms = lipgloss.NewStyle()
	}
	// Strip trailing HTTP version token for separate coloring.
	var version string
	if i := strings.LastIndex(rest, " "); i >= 0 {
		last := strings.ToUpper(strings.TrimSpace(rest[i+1:]))
		if last == "HTTP/1.0" || last == "HTTP/1.1" || last == "HTTP/2" || last == "HTTP/3" {
			version = last
			rest = strings.TrimSpace(rest[:i])
		}
	}
	out := ms.Bold(true).Render(method) + " " + colorizeTokens(rest, clrURL)
	if version != "" {
		out += " " + clrKeyword.Render(version)
	}
	return out
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

// colorizeTokens wraps s in base style, highlights {{var}} spans with clrVar,
// and highlights ${{expr}} spans (inline JS) with clrInlineExpr.
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
		end := strings.Index(s[start:], "}}")
		if end == -1 {
			// Unclosed token — render the rest as base text.
			b.WriteString(base.Render(s))
			break
		}
		end += start + 2

		// ${{expr}} — include the '$' prefix in the inline-expr span.
		isInline := start > 0 && s[start-1] == '$'
		tokenStart := start
		if isInline {
			tokenStart = start - 1
		}

		if tokenStart > 0 {
			b.WriteString(base.Render(s[:tokenStart]))
		}
		if isInline {
			b.WriteString(clrInlineExpr.Render(s[tokenStart:end]))
		} else {
			b.WriteString(clrVar.Render(s[start:end]))
		}
		s = s[end:]
	}
	return b.String()
}

// colorizeInlineExprs re-highlights ${{expr}} tokens inside an already-styled
// string (e.g. chroma output). Chroma tokenises JSON/XML bodies as whole string
// values, so the ${{...}} sequence appears verbatim within the ANSI span and can
// be replaced. The surrounding chroma color may break after the token, which is
// an acceptable trade-off for clear expression highlighting.
func colorizeInlineExprs(s string) string {
	if !strings.Contains(s, "${{") {
		return s
	}
	return inlineExprBodyRe.ReplaceAllStringFunc(s, func(match string) string {
		return clrInlineExpr.Render(match)
	})
}
