package domain

import "fmt"

// Provider identifies an edge platform ("cloudflare", "cloudfront", ...).
type Provider string

const ProviderCloudflare Provider = "cloudflare"

// ResourceRef points at one managed unit within a provider: a zone,
// distribution, service or site.
type ResourceRef struct {
	Provider   Provider `json:"provider"`
	Kind       string   `json:"kind"` // "zone", "distribution", ...
	ExternalID string   `json:"external_id"`
	Name       string   `json:"name"`
}

func (r ResourceRef) Validate() error {
	if r.Provider == "" || r.Kind == "" || r.ExternalID == "" {
		return fmt.Errorf("resource ref incomplete: provider=%q kind=%q external_id=%q", r.Provider, r.Kind, r.ExternalID)
	}
	return nil
}

// String is used in logs and audit lines; never includes credentials.
func (r ResourceRef) String() string {
	return fmt.Sprintf("%s/%s/%s(%s)", r.Provider, r.Kind, r.ExternalID, r.Name)
}
