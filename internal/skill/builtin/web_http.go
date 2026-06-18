package builtin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Web / HTTP skills — pure-stdlib (net/url, crypto/rand, sha256).
// No outbound network calls; everything is parsing/manipulation/generation.
//
// Skills:
//   - user_agent_parse
//   - url_query_extract
//   - url_query_build
//   - http_status_lookup
//   - mime_boundary_generate
//   - content_disposition_parse
//   - etag_generate
//   - range_parse
// =====================================================================================

// stripVerbColon is stripVerb followed by trimming any leading colon/whitespace
// the user typed between the verb and the operand (e.g. "etag: hello" → "hello").
// If the verb is not found in q, returns "" (so callers can try alt verbs).
func stripVerbColon(q, verb string) string {
	low := strings.ToLower(q)
	idx := strings.Index(low, strings.ToLower(verb))
	if idx < 0 {
		return ""
	}
	s := strings.TrimSpace(q[idx+len(verb):])
	s = strings.TrimPrefix(s, ":")
	return strings.TrimSpace(s)
}

// pickPayload tries each verb in turn (stripping a trailing colon), falling
// back to extractPayload. Returns "" only if every strategy yields empty.
func pickPayload(q string, verbs ...string) string {
	for _, v := range verbs {
		if s := stripVerbColon(q, v); s != "" {
			return s
		}
	}
	return extractPayload(q)
}

// ---- user_agent_parse ----

type UserAgentParseSkill struct{ *kyoci.BaseSkill }

func NewUserAgentParseSkill() *UserAgentParseSkill {
	return &UserAgentParseSkill{BaseSkill: kyoci.NewBaseSkill(
		"user_agent_parse", "Parse a User-Agent string into browser, version, OS, and device type",
		[]string{"user agent parse", "parse user agent", "user-agent"},
	)}
}
func (s *UserAgentParseSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "user agent parse") || strings.Contains(q, "parse user agent") ||
		strings.Contains(q, "user-agent parse") || strings.Contains(q, "parse user-agent")
}
func (s *UserAgentParseSkill) Execute(_ context.Context, q string) (string, error) {
	ua := pickPayload(q, "user agent parse", "parse user agent", "user-agent parse", "parse user-agent")
	ua = strings.TrimSpace(quoteStripped(ua))
	if ua == "" {
		return "", fmt.Errorf("no user-agent string provided")
	}
	browser, version := webUABrowser(ua)
	osName, osVersion := webUAOS(ua)
	device := webUADevice(ua)
	var b strings.Builder
	if browser != "" {
		if version != "" {
			fmt.Fprintf(&b, "Browser: %s %s\n", browser, version)
		} else {
			fmt.Fprintf(&b, "Browser: %s\n", browser)
		}
	}
	if osName != "" {
		if osVersion != "" {
			fmt.Fprintf(&b, "OS: %s %s\n", osName, osVersion)
		} else {
			fmt.Fprintf(&b, "OS: %s\n", osName)
		}
	}
	if device != "" {
		fmt.Fprintf(&b, "Device: %s\n", device)
	}
	out := strings.TrimRight(b.String(), "\n")
	if out == "" {
		return "", fmt.Errorf("could not parse user-agent: %s", ua)
	}
	return out, nil
}

// webUABrowser walks known browser signatures (most-specific first) and
// returns the first match's display name + version.
func webUABrowser(ua string) (string, string) {
	type sig struct {
		name    string
		pattern *regexp.Regexp
	}
	sigs := []sig{
		{"Edge", regexp.MustCompile(`Edge/(\d+(?:\.\d+)*)`)},
		{"Edge", regexp.MustCompile(`Edg/(\d+(?:\.\d+)*)`)},
		{"Opera", regexp.MustCompile(`OPR/(\d+(?:\.\d+)*)`)},
		{"Opera", regexp.MustCompile(`Opera/(\d+(?:\.\d+)*)`)},
		{"Firefox", regexp.MustCompile(`Firefox/(\d+(?:\.\d+)*)`)},
		{"Chrome", regexp.MustCompile(`Chrome/(\d+(?:\.\d+)*)`)},
		{"Safari", regexp.MustCompile(`Version/(\d+(?:\.\d+)*)`)},
		{"Safari", regexp.MustCompile(`Safari/(\d+(?:\.\d+)*)`)},
		{"Internet Explorer", regexp.MustCompile(`MSIE\s+(\d+(?:\.\d+)*)`)},
		{"Internet Explorer", regexp.MustCompile(`Trident/.*rv:(\d+(?:\.\d+)*)`)},
	}
	for _, s := range sigs {
		if m := s.pattern.FindStringSubmatch(ua); m != nil {
			return s.name, m[1]
		}
	}
	return "Unknown", ""
}

// webUAOS returns a friendly OS name and version derived from the parenthetical
// "compatibility" comment block of a UA string.
func webUAOS(ua string) (string, string) {
	switch {
	case strings.Contains(ua, "Windows NT"):
		ver := ""
		if m := regexp.MustCompile(`Windows NT (\d+\.\d+)`).FindStringSubmatch(ua); m != nil {
			switch m[1] {
			case "10.0":
				ver = "10/11"
			case "6.3":
				ver = "8.1"
			case "6.2":
				ver = "8"
			case "6.1":
				ver = "7"
			default:
				ver = m[1]
			}
		}
		return "Windows", ver
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		ver := ""
		if m := regexp.MustCompile(`Mac OS X (\d+)[_\.](\d+)(?:[_\.](\d+))?`).FindStringSubmatch(ua); m != nil {
			major, _ := strconv.Atoi(m[1])
			minor, _ := strconv.Atoi(m[2])
			patch := 0
			if m[3] != "" {
				patch, _ = strconv.Atoi(m[3])
			}
			ver = fmt.Sprintf("%d.%d.%d", major, minor, patch)
		}
		return "macOS", ver
	case strings.Contains(ua, "Android"):
		ver := ""
		if m := regexp.MustCompile(`Android (\d+(?:\.\d+)*)`).FindStringSubmatch(ua); m != nil {
			ver = m[1]
		}
		return "Android", ver
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad") || strings.Contains(ua, "iOS"):
		ver := ""
		if m := regexp.MustCompile(`OS (\d+)[_\.](\d+)`).FindStringSubmatch(ua); m != nil {
			major, _ := strconv.Atoi(m[1])
			minor, _ := strconv.Atoi(m[2])
			ver = fmt.Sprintf("%d.%d", major, minor)
		}
		return "iOS", ver
	case strings.Contains(ua, "Linux") || strings.Contains(ua, "X11"):
		return "Linux", ""
	case strings.Contains(ua, "CrOS"):
		return "ChromeOS", ""
	case strings.Contains(ua, "FreeBSD"):
		return "FreeBSD", ""
	}
	return "Unknown", ""
}

// webUADevice inspects the UA string for mobile/tablet/crawler hints.
func webUADevice(ua string) string {
	low := strings.ToLower(ua)
	switch {
	case strings.Contains(low, "bot") || strings.Contains(low, "crawler") ||
		strings.Contains(low, "spider") || strings.Contains(low, "slurp"):
		return "Bot"
	case strings.Contains(low, "ipad") || strings.Contains(low, "tablet") ||
		strings.Contains(low, "playbook") || strings.Contains(low, "kindle"):
		return "Tablet"
	case strings.Contains(low, "mobi") || strings.Contains(low, "android") ||
		strings.Contains(low, "iphone") || strings.Contains(low, "windows phone"):
		return "Mobile"
	default:
		return "Desktop"
	}
}

// ---- url_query_extract ----

type URLQueryExtractSkill struct{ *kyoci.BaseSkill }

func NewURLQueryExtractSkill() *URLQueryExtractSkill {
	return &URLQueryExtractSkill{BaseSkill: kyoci.NewBaseSkill(
		"url_query_extract", "Extract query parameters from a URL (one k=v per line)",
		[]string{"url query extract", "extract query", "url query"},
	)}
}
func (s *URLQueryExtractSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "url query extract") || strings.Contains(q, "extract query") ||
		strings.Contains(q, "extract url query")
}
func (s *URLQueryExtractSkill) Execute(_ context.Context, q string) (string, error) {
	raw := strings.TrimSpace(pickPayload(q, "url query extract", "extract query", "extract url query"))
	if raw == "" {
		return "", fmt.Errorf("no URL provided")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if u.RawQuery == "" {
		return "", fmt.Errorf("no query parameters in URL")
	}
	var out strings.Builder
	for _, pair := range strings.Split(u.RawQuery, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		decodedK, err := url.QueryUnescape(k)
		if err != nil {
			decodedK = k
		}
		decodedV, err := url.QueryUnescape(v)
		if err != nil {
			decodedV = v
		}
		fmt.Fprintf(&out, "%s=%s\n", decodedK, decodedV)
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// ---- url_query_build ----

type URLQueryBuildSkill struct{ *kyoci.BaseSkill }

func NewURLQueryBuildSkill() *URLQueryBuildSkill {
	return &URLQueryBuildSkill{BaseSkill: kyoci.NewBaseSkill(
		"url_query_build", "Build a URL with query parameters. Usage: 'url query build: <url> | k1=v1, k2=v2'",
		[]string{"url query build", "build url", "build url query"},
	)}
}
func (s *URLQueryBuildSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "url query build") || strings.Contains(q, "build url query")
}
func (s *URLQueryBuildSkill) Execute(_ context.Context, q string) (string, error) {
	payload := strings.TrimSpace(pickPayload(q, "url query build", "build url query"))
	if payload == "" {
		return "", fmt.Errorf("expected '<url> | k=v, k=v'")
	}
	base, params, found := strings.Cut(payload, "|")
	base = strings.TrimSpace(base)
	if !found {
		base, params, found = strings.Cut(payload, " ")
		base = strings.TrimSpace(base)
	}
	if base == "" {
		return "", fmt.Errorf("missing base URL")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	vals := url.Values{}
	for _, pair := range strings.Split(params, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return "", fmt.Errorf("invalid param %q (expected k=v)", pair)
		}
		vals.Set(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	u.RawQuery = vals.Encode()
	return u.String(), nil
}

// ---- http_status_lookup ----

type HTTPStatusLookupSkill struct{ *kyoci.BaseSkill }

func NewHTTPStatusLookupSkill() *HTTPStatusLookupSkill {
	return &HTTPStatusLookupSkill{BaseSkill: kyoci.NewBaseSkill(
		"http_status_lookup", "Look up the description and class of an HTTP status code",
		[]string{"http status", "http status lookup", "http status code"},
	)}
}
func (s *HTTPStatusLookupSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "http status") || strings.Contains(q, "status code")
}
func (s *HTTPStatusLookupSkill) Execute(_ context.Context, q string) (string, error) {
	// Match any 3-digit code 100-599 (per RFC 9110 §15). Codes outside the
	// table return "Unknown Status" with the right class hint.
	re := regexp.MustCompile(`\b([1-5]\d{2})\b`)
	m := re.FindStringSubmatch(q)
	if m == nil {
		return "", fmt.Errorf("no HTTP status code found")
	}
	code, err := strconv.Atoi(m[1])
	if err != nil {
		return "", fmt.Errorf("invalid status code: %s", m[1])
	}
	desc, ok := webHTTPStatusTable[code]
	if !ok {
		return fmt.Sprintf("%d Unknown Status (%s)", code, webHTTPStatusClass(code)), nil
	}
	return fmt.Sprintf("%d %s (%s)", code, desc, webHTTPStatusClass(code)), nil
}

// webHTTPStatusClass maps a status code's leading digit to its human class name.
func webHTTPStatusClass(code int) string {
	switch code / 100 {
	case 1:
		return "Informational"
	case 2:
		return "Success"
	case 3:
		return "Redirection"
	case 4:
		return "Client Error"
	case 5:
		return "Server Error"
	default:
		return "Unknown"
	}
}

// webHTTPStatusTable covers the common HTTP status codes from RFC 9110 + extensions.
var webHTTPStatusTable = map[int]string{
	100: "Continue",
	101: "Switching Protocols",
	102: "Processing",
	103: "Early Hints",
	200: "OK",
	201: "Created",
	202: "Accepted",
	203: "Non-Authoritative Information",
	204: "No Content",
	205: "Reset Content",
	206: "Partial Content",
	207: "Multi-Status",
	208: "Already Reported",
	226: "IM Used",
	300: "Multiple Choices",
	301: "Moved Permanently",
	302: "Found",
	303: "See Other",
	304: "Not Modified",
	305: "Use Proxy",
	307: "Temporary Redirect",
	308: "Permanent Redirect",
	400: "Bad Request",
	401: "Unauthorized",
	402: "Payment Required",
	403: "Forbidden",
	404: "Not Found",
	405: "Method Not Allowed",
	406: "Not Acceptable",
	407: "Proxy Authentication Required",
	408: "Request Timeout",
	409: "Conflict",
	410: "Gone",
	411: "Length Required",
	412: "Precondition Failed",
	413: "Payload Too Large",
	414: "URI Too Long",
	415: "Unsupported Media Type",
	416: "Range Not Satisfiable",
	417: "Expectation Failed",
	418: "I'm a Teapot",
	421: "Misdirected Request",
	422: "Unprocessable Entity",
	423: "Locked",
	424: "Failed Dependency",
	425: "Too Early",
	426: "Upgrade Required",
	428: "Precondition Required",
	429: "Too Many Requests",
	431: "Request Header Fields Too Large",
	451: "Unavailable For Legal Reasons",
	500: "Internal Server Error",
	501: "Not Implemented",
	502: "Bad Gateway",
	503: "Service Unavailable",
	504: "Gateway Timeout",
	505: "HTTP Version Not Supported",
	506: "Variant Also Negotiates",
	507: "Insufficient Storage",
	508: "Loop Detected",
	510: "Not Extended",
	511: "Network Authentication Required",
}

// ---- mime_boundary_generate ----

type MIMEBoundaryGenerateSkill struct{ *kyoci.BaseSkill }

func NewMIMEBoundaryGenerateSkill() *MIMEBoundaryGenerateSkill {
	return &MIMEBoundaryGenerateSkill{BaseSkill: kyoci.NewBaseSkill(
		"mime_boundary_generate", "Generate a random MIME multipart boundary string",
		[]string{"mime boundary", "boundary generate", "multipart boundary"},
	)}
}
func (s *MIMEBoundaryGenerateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "mime boundary") || strings.Contains(q, "boundary generate") ||
		strings.Contains(q, "multipart boundary")
}
func (s *MIMEBoundaryGenerateSkill) Execute(_ context.Context, _ string) (string, error) {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const length = 30
	max := big.NewInt(int64(len(alphabet)))
	var b strings.Builder
	b.WriteString("--")
	for i := 0; i < length-2; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("rand failed: %w", err)
		}
		b.WriteByte(alphabet[idx.Int64()])
	}
	return b.String(), nil
}

// ---- content_disposition_parse ----

type ContentDispositionParseSkill struct{ *kyoci.BaseSkill }

func NewContentDispositionParseSkill() *ContentDispositionParseSkill {
	return &ContentDispositionParseSkill{BaseSkill: kyoci.NewBaseSkill(
		"content_disposition_parse", "Parse a Content-Disposition header into type and filename",
		[]string{"content disposition", "parse content disposition", "content-disposition"},
	)}
}
func (s *ContentDispositionParseSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "content disposition") || strings.Contains(q, "content-disposition")
}
func (s *ContentDispositionParseSkill) Execute(_ context.Context, q string) (string, error) {
	header := strings.TrimSpace(quoteStripped(pickPayload(q,
		"content disposition parse", "parse content disposition", "content-disposition parse")))
	if header == "" {
		return "", fmt.Errorf("no Content-Disposition header provided")
	}
	parts := strings.SplitN(header, ";", 2)
	dispType := strings.TrimSpace(parts[0])
	if dispType == "" {
		return "", fmt.Errorf("missing disposition type")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Type: %s\n", strings.ToLower(dispType))
	if len(parts) == 2 {
		params := webParseHeaderParams(parts[1])
		if fn, ok := params["filename*"]; ok {
			fmt.Fprintf(&b, "Filename: %s\n", webDecodeExtValue(fn))
		} else if fn, ok := params["filename"]; ok {
			fmt.Fprintf(&b, "Filename: %s\n", fn)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// webParseHeaderParams parses "k1=\"v1\"; k2=v2" into a map.
func webParseHeaderParams(s string) map[string]string {
	out := map[string]string{}
	re := regexp.MustCompile(`([a-zA-Z\-\*]+)\s*=\s*(?:"([^"]*)"|([^;]+))`)
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		key := strings.ToLower(strings.TrimSpace(m[1]))
		val := m[2]
		if val == "" {
			val = strings.TrimSpace(m[3])
		}
		out[key] = val
	}
	return out
}

// webDecodeExtValue decodes an RFC 5987 "ext-value" (filename* param).
func webDecodeExtValue(raw string) string {
	parts := strings.SplitN(raw, "'", 3)
	if len(parts) != 3 {
		return raw
	}
	dec, err := url.QueryUnescape(parts[2])
	if err != nil {
		return parts[2]
	}
	return dec
}

// ---- etag_generate ----

type ETagGenerateSkill struct{ *kyoci.BaseSkill }

func NewETagGenerateSkill() *ETagGenerateSkill {
	return &ETagGenerateSkill{BaseSkill: kyoci.NewBaseSkill(
		"etag_generate", "Generate an ETag for input text (SHA-256, 16 bytes hex, quoted)",
		[]string{"etag", "generate etag", "entity tag"},
	)}
}
func (s *ETagGenerateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "etag")
}
func (s *ETagGenerateSkill) Execute(_ context.Context, q string) (string, error) {
	in := pickPayload(q, "generate etag", "etag")
	// pickPayload falls through to extractPayload which returns the bare
	// command word ("etag") when no operand is given — treat that as empty.
	if strings.EqualFold(strings.TrimSpace(in), "etag") {
		in = ""
	}
	in = strings.TrimSpace(in)
	if in == "" {
		return "", fmt.Errorf("no input text for ETag")
	}
	sum := sha256.Sum256([]byte(in))
	hexstr := hex.EncodeToString(sum[:16])
	return fmt.Sprintf("\"%s\"", hexstr), nil
}

// ---- range_parse ----

type RangeParseSkill struct{ *kyoci.BaseSkill }

func NewRangeParseSkill() *RangeParseSkill {
	return &RangeParseSkill{BaseSkill: kyoci.NewBaseSkill(
		"range_parse", "Parse an HTTP Range header (bytes=N-M / bytes=N- / bytes=-N)",
		[]string{"range parse", "http range", "parse range"},
	)}
}
func (s *RangeParseSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "range parse") || strings.Contains(q, "http range") ||
		strings.Contains(q, "parse range")
}
func (s *RangeParseSkill) Execute(_ context.Context, q string) (string, error) {
	raw := strings.TrimSpace(quoteStripped(pickPayload(q, "range parse", "http range", "parse range")))
	if raw == "" {
		return "", fmt.Errorf("no Range header provided")
	}
	// Allow "Range: bytes=0-499" form — strip a leading "Range:" if present.
	if low := strings.ToLower(raw); strings.HasPrefix(low, "range:") {
		raw = strings.TrimSpace(raw[len("range:"):])
	}
	parts := strings.SplitN(raw, "=", 2)
	if len(parts) != 2 || strings.ToLower(strings.TrimSpace(parts[0])) != "bytes" {
		return "", fmt.Errorf("unsupported range unit (only 'bytes' is supported): %s", raw)
	}
	ranges := strings.TrimSpace(parts[1])
	first := strings.Split(ranges, ",")[0]
	first = strings.TrimSpace(first)
	startStr, endStr, hasDash := strings.Cut(first, "-")
	if !hasDash {
		return "", fmt.Errorf("invalid range spec: %s", first)
	}
	startStr = strings.TrimSpace(startStr)
	endStr = strings.TrimSpace(endStr)
	switch {
	case startStr == "" && endStr != "":
		return fmt.Sprintf("Start: %s (suffix)", endStr), nil
	case startStr != "" && endStr == "":
		return fmt.Sprintf("Start: %s\nEnd: EOF", startStr), nil
	case startStr != "" && endStr != "":
		startN, err1 := strconv.Atoi(startStr)
		endN, err2 := strconv.Atoi(endStr)
		if err1 != nil || err2 != nil {
			return "", fmt.Errorf("invalid range bounds: %s", first)
		}
		if startN > endN {
			return "", fmt.Errorf("invalid range: start > end")
		}
		return fmt.Sprintf("Start: %d\nEnd: %d", startN, endN), nil
	default:
		return "", fmt.Errorf("invalid range spec: %s", first)
	}
}
