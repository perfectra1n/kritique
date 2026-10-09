package configfile

import (
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strings"

	"github.com/perfectra1n/kritique/internal/egress"
)

// GitHubHost is the forge every GitHub connection and GitHub sign-in talks
// to, and GitHubAPIHost its API, which a runner's gh calls.
const (
	GitHubHost    = "github.com"
	GitHubAPIHost = "api.github.com"
)

// validateEgress parses the allow and deny entries, checks that no deny
// entry cuts off the forge a connection needs, and that each credential
// names one host that is allowed, explicitly or implicitly; the gateway
// looks a credential up by exact host, so a pattern would never apply.
func (f *File) validateEgress() error {
	var err error
	if f.Egress.allow, err = parseList(f.Egress.Allow); err != nil {
		return fmt.Errorf("configfile: egress.allow%w", err)
	}
	if f.Egress.deny, err = parseList(f.Egress.Deny); err != nil {
		return fmt.Errorf("configfile: egress.deny%w", err)
	}
	rules := f.EgressRules()
	for _, host := range f.forgeHosts() {
		if !rules.Allows(host) {
			return fmt.Errorf("configfile: egress.deny: %s is denied, which an app needs", host)
		}
	}
	for _, host := range slices.Sorted(maps.Keys(f.Egress.credentials)) {
		if err := checkHost(host); err != nil {
			return fmt.Errorf("configfile: egress.credentials.%s: %w", host, err)
		}
		if strings.HasPrefix(host, "*") {
			return fmt.Errorf("configfile: egress.credentials.%s: a credential names one host, not a pattern", host)
		}
		if !rules.Allows(host) {
			return fmt.Errorf("configfile: egress.credentials.%s: host is not allowed by egress.allow and egress.deny", host)
		}
		if f.Egress.credentials[host].Value() == "" {
			return fmt.Errorf("configfile: egress.credentials.%s resolved to an empty value", host)
		}
	}
	return nil
}

// parseList sorts entries into hosts and address prefixes, an address
// alone as a prefix of its own length and an IPv4-mapped IPv6 entry as the
// IPv4 it maps, which is how the gateway sees such a destination; an error
// names the entry's index.
func parseList(entries []string) (egress.List, error) {
	var l egress.List
	for i, e := range entries {
		if p, err := netip.ParsePrefix(e); err == nil {
			if a := p.Addr(); a.Is4In6() && p.Bits() >= 96 {
				p = netip.PrefixFrom(a.Unmap(), p.Bits()-96)
			}
			l.Prefixes = append(l.Prefixes, p.Masked())
			continue
		}
		if a, err := netip.ParseAddr(e); err == nil {
			a = a.Unmap()
			l.Prefixes = append(l.Prefixes, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		if err := checkHost(e); err != nil {
			return egress.List{}, fmt.Errorf("[%d]: %w", i, err)
		}
		l.Hosts = append(l.Hosts, e)
	}
	return l, nil
}

// checkHost accepts a lowercase hostname, optionally with a leading "*.",
// or "*" alone, and nothing else: no scheme, port, path or trailing dot,
// which the gateway strips from a requested host, so an entry carrying one
// would never match.
func checkHost(h string) error {
	if h == "*" {
		return nil
	}
	bare := strings.TrimPrefix(h, "*.")
	if bare == "" || strings.ContainsAny(bare, "/:@ ") || strings.HasPrefix(bare, "*") ||
		strings.HasPrefix(bare, ".") || strings.HasSuffix(bare, ".") || h != strings.ToLower(h) {
		return fmt.Errorf("%q must be a lowercase hostname, optionally prefixed with \"*.\", \"*\" for every host, or an IP address or CIDR", h)
	}
	return nil
}

// forgeHosts are the hosts the connections' forges live on, which runners
// fetch from: GitHub and its API, which gh reads, once a GitHub connection
// exists, and each Forgejo or Gitea connection's host.
func (f *File) forgeHosts() []string {
	var hosts []string
	add := func(h string) {
		if h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	for i := range f.Connections {
		add(f.Connections[i].Host())
		if f.Connections[i].Forge == ForgeGitHub {
			add(GitHubAPIHost)
		}
	}
	return hosts
}

// EgressRules is what the gateway allows for this file: the configured
// entries and every connection's forge host (see forgeHosts), the denied
// entries, plus the credentials as Authorization header values. A forge
// host on a private address is reached only when an allow entry covers
// the address too, as any host is. Provider endpoints are not among them:
// a runner reaches its model through the gateway's model endpoint, and the
// worker calls the provider.
func (f *File) EgressRules() egress.Rules {
	allow := egress.List{Hosts: slices.Clone(f.Egress.allow.Hosts), Prefixes: f.Egress.allow.Prefixes}
	for _, h := range f.forgeHosts() {
		if !slices.Contains(allow.Hosts, h) {
			allow.Hosts = append(allow.Hosts, h)
		}
	}
	creds := make(map[string]string, len(f.Egress.credentials))
	for host, secret := range f.Egress.credentials {
		creds[host] = "Bearer " + secret.Value()
	}
	return egress.Rules{Allow: allow, Deny: f.Egress.deny, Credentials: creds}
}
