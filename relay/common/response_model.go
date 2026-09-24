package common

import "strings"

// ResponseModel records upstream declarations before response conversion. It is
// diagnostic only: it must never change routing, pricing, or downstream output.
type ResponseModel struct {
	RequestedModel string `json:"requested_model"`
	UpstreamModel  string `json:"upstream_model"`
	ReturnedModel  string `json:"returned_model"`
	Mismatch       bool   `json:"mismatch"`
	// Alias marks a returned name that matched only after stripping its
	// aggregator namespace prefix; it is logged under admin_info instead of the
	// user-visible scope.
	Alias bool `json:"alias,omitempty"`
}

// ObserveResponseModel retains the first differing model for inspection, with
// mismatches taking priority over prefix or case-only differences. A later
// matching or empty event cannot erase it. Only observe upstream declarations,
// never models synthesized by a response converter.
func (info *RelayInfo) ObserveResponseModel(model string) {
	if info == nil || strings.TrimSpace(model) == "" {
		return
	}
	if info.ResponseModel == nil {
		info.ResponseModel = &ResponseModel{
			RequestedModel: info.OriginModelName,
			UpstreamModel:  info.GetUpstreamModelName(),
		}
	}
	observation := info.ResponseModel
	if observation.Mismatch {
		return
	}
	// Aggregators may namespace the returned name ("devin/swe-2"); the basename
	// after the last "/" is checked with the same rule so a namespace alias is
	// not flagged as substitution. Expected names are never stripped, so a
	// swapped namespace ("other/llama-3" for "meta-llama/llama-3") still warns.
	base := model[strings.LastIndex(model, "/")+1:]
	mismatch := true
	alias := false
	for _, expected := range []string{observation.RequestedModel, observation.UpstreamModel} {
		if expected == "" {
			continue
		}
		if strings.HasPrefix(model, expected) || strings.EqualFold(model, expected) {
			mismatch = false
			alias = false
			break
		}
		if base != model && (strings.HasPrefix(base, expected) || strings.EqualFold(base, expected)) {
			mismatch = false
			alias = true
		}
	}
	if !mismatch && observation.ReturnedModel != "" &&
		observation.ReturnedModel != observation.RequestedModel && observation.ReturnedModel != observation.UpstreamModel {
		return
	}
	observation.ReturnedModel = model
	observation.Mismatch = mismatch
	observation.Alias = alias
}
