package builtin

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// cidrPattern matches an IPv4 CIDR like 192.168.1.0/24.
var cidrPattern = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/\d{1,2}\b`)

// SubnetSkill computes IPv4 subnet details from a CIDR.
type SubnetSkill struct {
	*kyoci.BaseSkill
}

// NewSubnetSkill creates a new subnet skill.
func NewSubnetSkill() *SubnetSkill {
	return &SubnetSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"subnet",
			"Compute IPv4 subnet details from a CIDR (e.g. 192.168.1.0/24)",
			[]string{"subnet", "cidr", "ip address", "network"},
		),
	}
}

// Match checks if the query is asking about a subnet/CIDR.
func (s *SubnetSkill) Match(query string) bool {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if strings.Contains(queryLower, "subnet") || strings.Contains(queryLower, "cidr") {
		return true
	}
	return cidrPattern.MatchString(query)
}

// Execute parses the CIDR and reports subnet details.
func (s *SubnetSkill) Execute(ctx context.Context, query string) (string, error) {
	cidr := cidrPattern.FindString(query)
	if cidr == "" {
		// Fallback: pick out any token containing "/".
		for _, f := range strings.Fields(query) {
			if strings.Contains(f, "/") {
				cidr = f
				break
			}
		}
	}
	if cidr == "" {
		return "", fmt.Errorf("could not find a CIDR in query: %s", query)
	}

	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	if ipnet.IP.To4() == nil {
		return "", fmt.Errorf("only IPv4 CIDRs are supported: %s", cidr)
	}

	ip4 := ipnet.IP.To4()
	mask := ipnet.Mask
	ones, bits := mask.Size()
	if bits != 32 {
		return "", fmt.Errorf("unexpected mask size %d for IPv4", bits)
	}

	// Network address: IP & mask.
	network := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		network[i] = ip4[i] & mask[i]
	}

	// Broadcast: network | ^mask.
	wildcard := net.IP(make([]byte, 4))
	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		wildcard[i] = ^mask[i]
		broadcast[i] = network[i] | wildcard[i]
	}

	totalHosts := uint64(1) << (32 - ones)
	usable := totalHosts
	if ones <= 30 {
		usable = totalHosts - 2
	}

	// First and last host (only meaningful for /30 or smaller).
	var firstHost, lastHost net.IP
	if ones <= 30 {
		firstHost = make(net.IP, 4)
		lastHost = make(net.IP, 4)
		copy(firstHost, network)
		copy(lastHost, broadcast)
		firstHost[3]++
		lastHost[3]--
	}

	var b strings.Builder
	fmt.Fprintf(&b, "CIDR: %s\n", cidr)
	fmt.Fprintf(&b, "Network address: %s\n", network.String())
	fmt.Fprintf(&b, "Broadcast address: %s\n", broadcast.String())
	fmt.Fprintf(&b, "Subnet mask: %s\n", net.IP(mask).String())
	fmt.Fprintf(&b, "Wildcard mask: %s\n", wildcard.String())
	fmt.Fprintf(&b, "Prefix length: /%d\n", ones)
	fmt.Fprintf(&b, "Total hosts: %d\n", totalHosts)
	if ones <= 30 {
		fmt.Fprintf(&b, "Usable hosts: %d\n", usable)
		fmt.Fprintf(&b, "Host range: %s - %s\n", firstHost.String(), lastHost.String())
	} else {
		// /31 and /32 have no host range in the classic sense.
		fmt.Fprintf(&b, "Usable hosts: %d\n", usable)
	}
	return b.String(), nil
}
