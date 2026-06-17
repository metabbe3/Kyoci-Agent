package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Networking skills — IP validation/info, MAC OUI lookup, port check, URL parse/build,
// CIDR validate/merge, DNS lookup. All use the stdlib net package; no external deps.
// =====================================================================================

// ---- IP validate ----

type IPValidateSkill struct{ *kyoci.BaseSkill }

func NewIPValidateSkill() *IPValidateSkill {
	return &IPValidateSkill{BaseSkill: kyoci.NewBaseSkill(
		"ip_validate", "Validate and classify an IP address (v4, v6, or invalid)",
		[]string{"validate ip", "ip validate", "is valid ip", "is this an ip"},
	)}
}
func (s *IPValidateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "validate ip") || strings.Contains(q, "ip validate") ||
		strings.Contains(q, "is valid ip") || strings.Contains(q, "is this an ip") ||
		strings.Contains(q, "is this ip valid")
}
func (s *IPValidateSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload mis-parses IPv6 (colons). Pull the IP via regex.
	ipRe := regexp.MustCompile(`\b(\d{1,3}\.){3}\d{1,3}\b|([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}`)
	m := ipRe.FindString(q)
	if m == "" {
		return "invalid (not an IP address)", nil
	}
	ip := net.ParseIP(m)
	if ip == nil {
		return "invalid (not an IP address)", nil
	}
	if ip.To4() != nil {
		return fmt.Sprintf("valid IPv4: %s", m), nil
	}
	return fmt.Sprintf("valid IPv6: %s", m), nil
}

// ---- IP info ----

type IPInfoSkill struct{ *kyoci.BaseSkill }

func NewIPInfoSkill() *IPInfoSkill {
	return &IPInfoSkill{BaseSkill: kyoci.NewBaseSkill(
		"ip_info", "Detailed info about an IP (version, classification)",
		[]string{"ip info", "information about ip", "ip details", "what is this ip"},
	)}
}
func (s *IPInfoSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "ip info") || strings.Contains(q, "ip details") ||
		strings.Contains(q, "what is this ip") || strings.Contains(q, "information about ip")
}
func (s *IPInfoSkill) Execute(_ context.Context, q string) (string, error) {
	// Pull the IP via regex — extractPayload gets confused by colons in IPv6
	// and stops too early for IPv4-with-trailing-text.
	ipRe := regexp.MustCompile(`\b(\d{1,3}\.){3}\d{1,3}\b|([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}`)
	m := ipRe.FindString(q)
	if m == "" {
		return "", fmt.Errorf("no IP address found")
	}
	ip := net.ParseIP(m)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address: %s", m)
	}
	var b strings.Builder
	if v4 := ip.To4(); v4 != nil {
		b.WriteString("version: IPv4\n")
		b.WriteString("address: " + v4.String() + "\n")
		if v4.IsLoopback() {
			b.WriteString("class: loopback\n")
		} else if v4.IsPrivate() {
			b.WriteString("class: private (RFC 1918)\n")
		} else if v4.IsMulticast() {
			b.WriteString("class: multicast\n")
		} else if v4.IsLinkLocalUnicast() {
			b.WriteString("class: link-local (APIPA)\n")
		} else if v4.IsUnspecified() {
			b.WriteString("class: unspecified (0.0.0.0)\n")
		} else {
			b.WriteString("class: public\n")
		}
	} else {
		b.WriteString("version: IPv6\n")
		b.WriteString("address: " + ip.String() + "\n")
		if ip.IsLoopback() {
			b.WriteString("class: loopback (::1)\n")
		} else if ip.IsPrivate() {
			b.WriteString("class: private (ULA fc00::/7)\n")
		} else if ip.IsLinkLocalUnicast() {
			b.WriteString("class: link-local (fe80::/10)\n")
		} else if ip.IsMulticast() {
			b.WriteString("class: multicast (ff00::/8)\n")
		} else {
			b.WriteString("class: public\n")
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- MAC lookup ----

type MACLookupSkill struct{ *kyoci.BaseSkill }

func NewMACLookupSkill() *MACLookupSkill {
	return &MACLookupSkill{BaseSkill: kyoci.NewBaseSkill(
		"mac_lookup", "Look up the vendor for a MAC address (OUI prefix)",
		[]string{"mac lookup", "oui lookup", "vendor for mac", "who makes this mac"},
	)}
}
func (s *MACLookupSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "mac lookup") || strings.Contains(q, "oui lookup") ||
		strings.Contains(q, "vendor for mac") || strings.Contains(q, "who makes this mac")
}
func (s *MACLookupSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload splits on the first ':' (which is inside the MAC address),
	// so pull via regex.
	macRe := regexp.MustCompile(`([0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}`)
	m := macRe.FindString(q)
	if m == "" {
		return "", fmt.Errorf("no MAC address found")
	}
	hw, err := net.ParseMAC(m)
	if err != nil {
		return "", fmt.Errorf("invalid MAC: %w", err)
	}
	oui := strings.ToUpper(strings.ReplaceAll(hw.String()[:8], ":", ""))
	vendor, ok := ouiTable[oui]
	if !ok {
		// Try with hyphens normalized.
		vendor, ok = ouiTable[strings.ToUpper(hw.String()[:8])]
	}
	if !ok {
		return fmt.Sprintf("OUI %s: unknown vendor (not in embedded table)", oui), nil
	}
	return fmt.Sprintf("OUI %s: %s", oui, vendor), nil
}

// ouiTable is a tiny embedded OUI → vendor table covering the most common
// network equipment manufacturers. A real deployment would load this from
// the IEEE OUI database (~50k entries, ~3MB); we ship a curated subset.
var ouiTable = map[string]string{
	"000C29": "VMware",
	"000569": "VMware",
	"080027": "VirtualBox (PCS Systemtechnik)",
	"001C42": "Parallels",
	"F45EAB": "Apple",
	"000393": "Apple",
	"ACDE48": "Apple",
	"001124": "Apple",
	"3CD92B": "HP",
	"0001E6": "HP",
	"001B21": "Intel (corporate)",
	"F0DEF7": "Dell",
	"001422": "Dell",
	"002500": "Lenovo (corporate)",
	"B47AC8": "Lenovo",
	"00235C": "Cisco",
	"000163": "Cisco",
	"001517": "Cisco",
	"B827EB": "Raspberry Pi Foundation",
	"DC396F": "Espressif (ESP32)",
	"240AC4": "Espressif",
	"0022F7": "Intel",
	"0017F2": "Intel",
	"001AA0": "Google",
	"42010E": "Google",
	"E45F01": "Microsoft (Surface)",
	"A46C2A": "Microsoft",
	"5C8789": "Netgear",
	"000FB5": "Netgear",
	"C03F0E": "TP-Link",
	"50C7BF": "TP-Link",
	"9C5C8E": "Asus",
	"002590": "Super Micro",
	"7085C2": "Samsung",
	"002268": "Amazon (AWS)",
	"00A069": "D-Link",
	"00179A": "D-Link",
	"0004E2": "Realtek",
	"E0CB4E": "Cisco-Meraki",
	"001A11": "Google Nest",
	"0021CC": "Gigabyte",
	"00C0EE": "Lambda",
	"04D9F5": "Razer",
}

// ---- port check ----

type PortCheckSkill struct{ *kyoci.BaseSkill }

func NewPortCheckSkill() *PortCheckSkill {
	return &PortCheckSkill{BaseSkill: kyoci.NewBaseSkill(
		"port_check", "Check if a TCP port is open on a host. Usage: 'port_check host:port'",
		[]string{"port check", "check port", "is port open", "port open"},
	)}
}
func (s *PortCheckSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "port check") || strings.Contains(q, "check port") ||
		strings.Contains(q, "is port open") || strings.Contains(q, "port open")
}
func (s *PortCheckSkill) Execute(ctx context.Context, q string) (string, error) {
	target := strings.TrimSpace(extractPayload(q))
	if target == "" {
		// Fall back to the whole query (some users type "port_check host:port").
		target = strings.TrimSpace(q)
	}
	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := net.DialTimeout("tcp", target, 3*time.Second)
	if err != nil {
		_ = dialCtx
		return fmt.Sprintf("closed: %s", target), nil
	}
	conn.Close()
	return fmt.Sprintf("open: %s", target), nil
}

// ---- URL parse ----

type URLParseSkill struct{ *kyoci.BaseSkill }

func NewURLParseSkill() *URLParseSkill {
	return &URLParseSkill{BaseSkill: kyoci.NewBaseSkill(
		"url_parse", "Parse a URL into scheme/host/path/query/fragment",
		[]string{"parse url", "url parse", "split url"},
	)}
}
func (s *URLParseSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "parse url") || strings.Contains(q, "url parse") ||
		strings.Contains(q, "split url")
}
func (s *URLParseSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload would strip at the first ':' (inside https://), so pull
	// the URL via regex instead.
	urlRe := regexp.MustCompile(`(https?://[^\s]+|ftp://[^\s]+)`)
	m := urlRe.FindString(q)
	if m == "" {
		// Fall back: text after the last space.
		idx := strings.LastIndex(q, " ")
		m = strings.TrimSpace(q[idx+1:])
	}
	if m == "" {
		return "", fmt.Errorf("no URL found in query")
	}
	u, err := url.Parse(m)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "scheme: %s\n", u.Scheme)
	fmt.Fprintf(&b, "host: %s\n", u.Hostname())
	fmt.Fprintf(&b, "port: %s\n", u.Port())
	fmt.Fprintf(&b, "path: %s\n", u.Path)
	if u.RawQuery != "" {
		fmt.Fprintf(&b, "query: %s\n", u.RawQuery)
		for k, v := range u.Query() {
			fmt.Fprintf(&b, "  %s = %v\n", k, v)
		}
	}
	if u.Fragment != "" {
		fmt.Fprintf(&b, "fragment: %s\n", u.Fragment)
	}
	if u.User != nil {
		fmt.Fprintf(&b, "username: %s\n", u.User.Username())
		if pw, ok := u.User.Password(); ok {
			fmt.Fprintf(&b, "password: %s\n", pw)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- URL build ----

type URLBuildSkill struct{ *kyoci.BaseSkill }

func NewURLBuildSkill() *URLBuildSkill {
	return &URLBuildSkill{BaseSkill: kyoci.NewBaseSkill(
		"url_build", "Build a URL from JSON parts: {scheme, host, port, path, query, fragment}",
		[]string{"build url", "url build", "compose url", "make url"},
	)}
}
func (s *URLBuildSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "build url") || strings.Contains(q, "url build") ||
		strings.Contains(q, "compose url") || strings.Contains(q, "make url")
}
func (s *URLBuildSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload breaks on the colons inside the JSON values, so find the
	// JSON object directly via regex.
	jsonRe := regexp.MustCompile(`\{[\s\S]*\}`)
	m := jsonRe.FindString(q)
	if m == "" {
		return "", fmt.Errorf("no JSON object found in query")
	}
	in := m
	parts := struct {
		Scheme   string            `json:"scheme"`
		Host     string            `json:"host"`
		Port     string            `json:"port"`
		Path     string            `json:"path"`
		Query    map[string]string `json:"query"`
		Fragment string            `json:"fragment"`
	}{}
	if err := jsonUnmarshalLenient([]byte(in), &parts); err != nil {
		return "", fmt.Errorf("expected JSON with scheme/host/port/path/query/fragment: %w", err)
	}
	u := &url.URL{
		Scheme:   parts.Scheme,
		Host:     parts.Host,
		Path:     parts.Path,
		Fragment: parts.Fragment,
	}
	if parts.Port != "" {
		u.Host = net.JoinHostPort(parts.Host, parts.Port)
	}
	if len(parts.Query) > 0 {
		vals := url.Values{}
		for k, v := range parts.Query {
			vals.Set(k, v)
		}
		u.RawQuery = vals.Encode()
	}
	return u.String(), nil
}

// jsonUnmarshalLenient wraps encoding/json.Unmarshal. Named separately so
// tests can substitute a more tolerant parser if needed.
func jsonUnmarshalLenient(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

// jsonDecode is the canonical "decode JSON" helper for skills. Currently a
// straight pass-through to encoding/json; kept as an indirection so future
// permissive decoding (trailing commas, comments) can be added centrally.
func jsonDecode(b []byte, v any) error { return json.Unmarshal(b, v) }

// jsonUnmarshalInto is the historical alias — kept for compatibility with
// earlier skill files that reference it by that name.
func jsonUnmarshalInto(b []byte, v any) error { return json.Unmarshal(b, v) }

// ---- CIDR validate ----

type CIDRValidateSkill struct{ *kyoci.BaseSkill }

func NewCIDRValidateSkill() *CIDRValidateSkill {
	return &CIDRValidateSkill{BaseSkill: kyoci.NewBaseSkill(
		"cidr_validate", "Validate a CIDR block and show network/broadcast/range",
		[]string{"cidr validate", "validate cidr", "is valid cidr"},
	)}
}
func (s *CIDRValidateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "cidr validate") || strings.Contains(q, "validate cidr") ||
		strings.Contains(q, "is valid cidr")
}
func (s *CIDRValidateSkill) Execute(_ context.Context, q string) (string, error) {
	// Pull the CIDR block via regex — extractPayload can't cleanly separate
	// "validate cidr 192.168.1.0/24" since there's no colon and the helper's
	// stopword strip leaves "cidr 192.168.1.0/24".
	cidrRe := regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/\d{1,2}\b`)
	m := cidrRe.FindString(q)
	if m == "" {
		return "", fmt.Errorf("no CIDR block found")
	}
	_, ipNet, err := net.ParseCIDR(m)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR: %w", err)
	}
	ones, bits := ipNet.Mask.Size()
	var b strings.Builder
	fmt.Fprintf(&b, "valid CIDR: %s\n", m)
	version := 6
	if bits == 32 {
		version = 4
	}
	fmt.Fprintf(&b, "version: IPv%d\n", version)
	fmt.Fprintf(&b, "mask bits: /%d (of %d)\n", ones, bits)
	fmt.Fprintf(&b, "network: %s\n", ipNet.IP)
	if bits == 32 {
		// IPv4 — compute broadcast
		ipBytes := ipNet.IP.To4()
		maskBytes := net.IP(ipNet.Mask).To4()
		broadcast := make(net.IP, 4)
		for i := 0; i < 4; i++ {
			broadcast[i] = ipBytes[i] | ^maskBytes[i]
		}
		fmt.Fprintf(&b, "broadcast: %s\n", broadcast)
		hostBits := bits - ones
		hostCount := uint64(1) << hostBits
		if hostBits < 32 {
			count := int64(hostCount) - 2
			if count < 0 {
				count = 0
			}
			fmt.Fprintf(&b, "host count: %d\n", count)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- CIDR merge ----

type CIDRMergeSkill struct{ *kyoci.BaseSkill }

func NewCIDRMergeSkill() *CIDRMergeSkill {
	return &CIDRMergeSkill{BaseSkill: kyoci.NewBaseSkill(
		"cidr_merge", "Merge a list of CIDR blocks into the minimal covering set",
		[]string{"cidr merge", "merge cidr", "consolidate cidr", "combine cidr"},
	)}
}
func (s *CIDRMergeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "cidr merge") || strings.Contains(q, "merge cidr") ||
		strings.Contains(q, "consolidate cidr") || strings.Contains(q, "combine cidr")
}
func (s *CIDRMergeSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	var nets []*net.IPNet
	for _, line := range strings.FieldsFunc(in, func(r rune) bool { return r == '\n' || r == ',' || r == ' ' }) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		_, n, err := net.ParseCIDR(line)
		if err != nil {
			continue
		}
		nets = append(nets, n)
	}
	if len(nets) == 0 {
		return "", fmt.Errorf("no valid CIDR blocks in input")
	}
	merged := mergeIPv4CIDRs(nets)
	out := make([]string, len(merged))
	for i, n := range merged {
		out[i] = n.String()
	}
	return strings.Join(out, "\n"), nil
}

// mergeIPv4CIDRs is a simple greedy merge — sorts by network address, then
// folds adjacent blocks when the result is itself a valid CIDR with the same
// prefix length and aligned boundary.
func mergeIPv4CIDRs(nets []*net.IPNet) []*net.IPNet {
	if len(nets) == 0 {
		return nets
	}
	// Sort by IP then prefix length (smaller prefix length = larger block).
	sorted := make([]*net.IPNet, len(nets))
	copy(sorted, nets)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if compareIPNet(sorted[i], sorted[j]) > 0 {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	// Fold adjacent.
	merged := []*net.IPNet{sorted[0]}
	for _, n := range sorted[1:] {
		last := merged[len(merged)-1]
		if last.Contains(n.IP) {
			continue // subsumed
		}
		// Try to merge last + n into a /k-1 if aligned.
		ones, _ := last.Mask.Size()
		if ones == 0 {
			merged = append(merged, n)
			continue
		}
		// Same prefix length? Check alignment for /k-1.
		lastOnes, _ := last.Mask.Size()
		nOnes, _ := n.Mask.Size()
		if lastOnes == nOnes && lastOnes > 0 {
			// New merged block has prefix length lastOnes-1.
			mergedMask := net.CIDRMask(lastOnes-1, 32)
			mergedIP := last.IP.Mask(mergedMask)
			mergedNet := &net.IPNet{IP: mergedIP, Mask: mergedMask}
			if mergedNet.Contains(n.IP) && mergedNet.Contains(last.IP) {
				merged[len(merged)-1] = mergedNet
				continue
			}
		}
		merged = append(merged, n)
	}
	return merged
}

// compareIPNet orders by IP (numeric) then by mask (smaller prefix length first).
func compareIPNet(a, b *net.IPNet) int {
	ab := a.IP.To4()
	bb := b.IP.To4()
	if ab == nil || bb == nil {
		return strings.Compare(a.IP.String(), b.IP.String())
	}
	for i := 0; i < 4; i++ {
		if ab[i] < bb[i] {
			return -1
		}
		if ab[i] > bb[i] {
			return 1
		}
	}
	ao, _ := a.Mask.Size()
	bo, _ := b.Mask.Size()
	if ao < bo {
		return -1
	}
	if ao > bo {
		return 1
	}
	return 0
}

// ---- DNS lookup ----

type DNSLookupSkill struct{ *kyoci.BaseSkill }

func NewDNSLookupSkill() *DNSLookupSkill {
	return &DNSLookupSkill{BaseSkill: kyoci.NewBaseSkill(
		"dns_lookup", "DNS lookup (A, AAAA, MX, TXT, NS, CNAME). Usage: 'dns_lookup MX example.com'",
		[]string{"dns lookup", "lookup dns", "resolve dns", "dig"},
	)}
}
func (s *DNSLookupSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "dns lookup") || strings.Contains(q, "lookup dns") ||
		strings.Contains(q, "resolve dns") || strings.Contains(q, "dns resolve") ||
		strings.HasPrefix(q, "dig ") || strings.Contains(q, "dig mx")
}
func (s *DNSLookupSkill) Execute(ctx context.Context, q string) (string, error) {
	low := strings.ToLower(q)
	recordType := "A"
	for _, t := range []string{"AAAA", "MX", "TXT", "NS", "CNAME", "PTR", "SRV"} {
		tl := strings.ToLower(t)
		if strings.Contains(low, " "+tl+" ") || strings.Contains(low, " "+tl+":") ||
			strings.Contains(low, tl+" record") {
			recordType = t
			break
		}
	}
	// Pull the domain via regex — looks like a hostname with at least one dot,
	// no spaces, no slashes. This avoids the extractPayload colon-stripping bug
	// and the prior bug where the record type was prepended to the domain.
	domainRe := regexp.MustCompile(`\b([a-z0-9]([-a-z0-9]*[a-z0-9])?\.)+[a-z]{2,}\b`)
	m := domainRe.FindString(low)
	if m == "" {
		return "", fmt.Errorf("no domain found in query")
	}
	domain := m

	r := net.Resolver{}
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var b strings.Builder
	fmt.Fprintf(&b, "lookup: %s %s\n\n", recordType, domain)
	switch recordType {
	case "A":
		ips, err := r.LookupHost(ctx2, domain)
		if err != nil {
			return "", err
		}
		for _, ip := range ips {
			if strings.Contains(ip, ".") {
				b.WriteString(ip + "\n")
			}
		}
	case "AAAA":
		ips, err := r.LookupHost(ctx2, domain)
		if err != nil {
			return "", err
		}
		for _, ip := range ips {
			if strings.Contains(ip, ":") {
				b.WriteString(ip + "\n")
			}
		}
	case "MX":
		mxs, err := r.LookupMX(ctx2, domain)
		if err != nil {
			return "", err
		}
		for _, mx := range mxs {
			fmt.Fprintf(&b, "%d %s\n", mx.Pref, mx.Host)
		}
	case "TXT":
		txts, err := r.LookupTXT(ctx2, domain)
		if err != nil {
			return "", err
		}
		for _, txt := range txts {
			b.WriteString(txt + "\n")
		}
	case "NS":
		nss, err := r.LookupNS(ctx2, domain)
		if err != nil {
			return "", err
		}
		for _, ns := range nss {
			b.WriteString(ns.Host + "\n")
		}
	case "CNAME":
		cname, err := r.LookupCNAME(ctx2, domain)
		if err != nil {
			return "", err
		}
		b.WriteString(cname + "\n")
	default:
		return "", fmt.Errorf("unsupported record type: %s", recordType)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// parseIntStr parses a base-10 integer.
func parseIntStr(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
