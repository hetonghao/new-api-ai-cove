package common

import "strings"

// ResponseModel records upstream declarations before response conversion. It is
// diagnostic only: it must never change routing, pricing, or downstream output.
// Only the three names are stored; whether they disagree is computed on demand
// so every consumer applies the current comparison rule to old rows as well.
type ResponseModel struct {
	RequestedModel string `json:"requested_model"`
	UpstreamModel  string `json:"upstream_model"`
	ReturnedModel  string `json:"returned_model"`
	Alias          bool   `json:"alias,omitempty"`
}

func normalizeResponseModelName(model string) string {
	return strings.ReplaceAll(strings.ToLower(model), ".", "-")
}

// matches reports whether an upstream declaration is compatible with the
// requested or upstream model: equal ignoring case and dot/hyphen differences,
// a dated or variant name that extends it, or the same name behind a provider
// path such as
// "deepseek/deepseek-v4.1-flash".
func (r *ResponseModel) matches(model string) bool {
	returned := normalizeResponseModelName(model)
	for _, expected := range []string{r.RequestedModel, r.UpstreamModel} {
		expected = normalizeResponseModelName(expected)
		if expected != "" && (strings.HasPrefix(returned, expected) || strings.HasPrefix(returned[strings.LastIndex(returned, "/")+1:], expected)) {
			return true
		}
	}
	return false
}

// Mismatch reports whether the retained upstream declaration disagrees with
// both the requested and upstream models.
func (r *ResponseModel) Mismatch() bool {
	return r != nil && r.ReturnedModel != "" && !r.matches(r.ReturnedModel)
}

// ObserveResponseModel retains the first differing model for inspection, with
// mismatches taking priority over provider-path, prefix, or case-only
// differences. A later matching or empty event cannot erase it. Only observe
// upstream declarations, never models synthesized by a response converter.
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
	if observation.Mismatch() {
		return
	}
	if observation.matches(model) && observation.ReturnedModel != "" &&
		observation.ReturnedModel != observation.RequestedModel && observation.ReturnedModel != observation.UpstreamModel {
		return
	}
	observation.ReturnedModel = model
	observation.Alias = observation.matches(model) && !strings.HasPrefix(normalizeResponseModelName(model), normalizeResponseModelName(observation.RequestedModel)) && (observation.UpstreamModel == "" || !strings.HasPrefix(normalizeResponseModelName(model), normalizeResponseModelName(observation.UpstreamModel)))
}
