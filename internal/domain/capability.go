package domain

// Capability is a discrete feature a connector declares, in "area:verb"
// form (see CONNECTORS.md). The platform degrades gracefully around
// missing capabilities instead of failing.
type Capability string

const (
	CapDNSRead        Capability = "dns:read"
	CapDNSWrite       Capability = "dns:write"
	CapWAFRead        Capability = "waf:read"
	CapWAFWrite       Capability = "waf:write"
	CapCacheRead      Capability = "cache:read"
	CapCachePurge     Capability = "cache:purge"
	CapCacheRules     Capability = "cache:rules"
	CapRatelimitRead  Capability = "ratelimit:read"
	CapRatelimitWrite Capability = "ratelimit:write"
	CapAnalyticsRead  Capability = "analytics:read"
	CapEventsRead     Capability = "events:read"
	CapEventsStream   Capability = "events:stream"
	CapSSLRead        Capability = "ssl:read"
	CapBotRead        Capability = "bot:read"
	CapLogsStream     Capability = "logs:stream"
	CapConfigRead     Capability = "config:read"
)

// HasCapability reports whether set contains c.
func HasCapability(set []Capability, c Capability) bool {
	for _, s := range set {
		if s == c {
			return true
		}
	}
	return false
}
