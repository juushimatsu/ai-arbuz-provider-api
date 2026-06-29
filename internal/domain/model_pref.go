package domain

// ModelPreference pins a specific upstream key as the preferred one for a model
// within a provider. Used to disambiguate when several keys serve the same
// model. Routing still fails over to other usable keys if the preferred one is
// down — the preference only controls priority, never exclusivity.
type ModelPreference struct {
	ProviderID    ID     `json:"provider_id"`
	Model         string `json:"model"`
	UpstreamKeyID ID     `json:"upstream_key_id"`
}
