package k8s

import (
	"fmt"
	"maps"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"github.com/Soo-de/suiro/internal/model"
)

// The check functions below follow the API server's validation. Each takes
// the locator of the value it checks and returns the locator of the first
// field that fails, or "" when the value is valid.

var (
	qualifiedName = regexp.MustCompile(`^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$`)
	dnsSubdomain  = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	dnsLabel      = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	serviceName   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

const (
	maxLabelLength     = 63
	maxSubdomainLength = 253
	maxPortNameLength  = 15
	maxPort            = 65535
)

func validLabelKey(key string) bool {
	name := key
	if prefix, rest, prefixed := strings.Cut(key, "/"); prefixed {
		if len(prefix) > maxSubdomainLength || !dnsSubdomain.MatchString(prefix) {
			return false
		}
		name = rest
	}
	return len(name) <= maxLabelLength && qualifiedName.MatchString(name)
}

func validLabelValue(value string) bool {
	return value == "" || len(value) <= maxLabelLength && qualifiedName.MatchString(value)
}

func validDNSLabel(s string) bool { return len(s) <= maxLabelLength && dnsLabel.MatchString(s) }

// validPortName reports whether s is an IANA service name: short, lowercase,
// with at least one letter and single hyphens inside it only.
func validPortName(s string) bool {
	return len(s) <= maxPortNameLength && serviceName.MatchString(s) &&
		strings.ContainsFunc(s, func(r rune) bool { return r >= 'a' && r <= 'z' })
}

func validPort(n int64) bool { return n >= 1 && n <= maxPort }

// portNumber converts a number validation has bounded to 0-65535.
func portNumber(n int64) uint16 {
	return uint16(n) //nolint:gosec // bounded by the caller's validation
}

// modelProtocol returns the model's name of a Kubernetes protocol, and false
// for SCTP, which is outside the model.
func modelProtocol(protocol string) (model.Protocol, bool) {
	switch protocol {
	case "TCP":
		return model.ProtocolTCP, true
	case "UDP":
		return model.ProtocolUDP, true
	}
	return "", false
}

// validProtocol reports whether protocol is one the API accepts.
func validProtocol(protocol string) bool {
	_, modeled := modelProtocol(protocol)
	return modeled || protocol == "SCTP"
}

// checkLabels checks a map of labels; the API names the map, not the key.
func checkLabels(labels map[string]string, locator string) string {
	// Order does not matter: every failure has the same locator.
	for _, key := range slices.Collect(maps.Keys(labels)) {
		if !validLabelKey(key) || !validLabelValue(labels[key]) {
			return locator
		}
	}
	return ""
}

// checkSelector checks a label selector; nil is valid.
func checkSelector(s *model.LabelSelector, locator string) string {
	if s == nil {
		return ""
	}
	if bad := checkLabels(s.MatchLabels, locator+".matchLabels"); bad != "" {
		return bad
	}
	for i, r := range s.MatchExpressions {
		if bad := checkRequirement(r, fmt.Sprintf("%s.matchExpressions[%d]", locator, i)); bad != "" {
			return bad
		}
	}
	return ""
}

func checkRequirement(r model.LabelRequirement, locator string) string {
	switch r.Operator {
	case "In", "NotIn":
		if len(r.Values) == 0 {
			return locator + ".values"
		}
	case "Exists", "DoesNotExist":
		if len(r.Values) > 0 {
			return locator + ".values"
		}
	default:
		return locator + ".operator"
	}
	if !validLabelKey(r.Key) {
		return locator + ".key"
	}
	for i, v := range r.Values {
		if !validLabelValue(v) {
			return fmt.Sprintf("%s.values[%d]", locator, i)
		}
	}
	return ""
}

// parseCIDR returns s as a masked prefix, and false when s is not a CIDR.
func parseCIDR(s string) (netip.Prefix, bool) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, false
	}
	return p.Masked(), true
}

// strictSubset reports whether inner lies inside outer and is smaller.
func strictSubset(inner, outer netip.Prefix) bool {
	return inner.Bits() > outer.Bits() && outer.Contains(inner.Addr())
}
