package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

const mediaPolicyMaxBodyBytes = 1 << 20

var mediaOperationMediaType = map[string]string{
	"text_to_image":  "image",
	"image_to_image": "image",
	"text_to_video":  "video",
	"image_to_video": "video",
}

var mediaDefaultOperation = map[string]string{
	"image": "text_to_image",
	"video": "text_to_video",
}

type mediaAvailabilityWire struct {
	Routable bool   `json:"routable"`
	Health   string `json:"health"`
}

type mediaCatalogItem struct {
	ID            string                          `json:"id"`
	Type          string                          `json:"type"`
	Operations    map[string]model.MediaOperation `json:"operations"`
	Availability  mediaAvailabilityWire           `json:"availability"`
	SelectionHint string                          `json:"selection_hint,omitempty"`
}

type mediaDefaultEntry struct {
	Model      *string `json:"model"`
	ReasonCode string  `json:"reason_code"`
}

type mediaPolicyUpdateRequest struct {
	ExpectedVersion string          `json:"expected_version"`
	Policy          json.RawMessage `json:"policy"`
}

var mediaPresetOpenAIImageIDs = map[string]bool{
	"gpt-image-1":      true,
	"gpt-image-1-mini": true,
	"gpt-image-1.5":    true,
	"gpt-image-2":      true,
}

var mediaPresetGeminiImageIDs = map[string]bool{
	"gemini-2.5-flash-image":         true,
	"gemini-3-pro-image":             true,
	"gemini-3-pro-image-preview":     true,
	"gemini-3.1-flash-image":         true,
	"gemini-3.1-flash-image-preview": true,
}

var mediaGeminiAspectRatio = model.MediaParameter{
	Type: "string",
	Enum: []string{"1:1", "16:9", "9:16"},
}

var mediaGeminiResolution = model.MediaParameter{
	Type: "string",
	Enum: []string{"1K", "2K", "4K"},
}

func mediaNoReference() model.MediaReference {
	return model.MediaReference{Input: "none", MaxImages: 0}
}

func mediaInlineReference() model.MediaReference {
	return model.MediaReference{Input: "inline", MaxImages: 1}
}

func mediaGeminiParameters(id string) map[string]model.MediaParameter {
	parameters := map[string]model.MediaParameter{
		"aspect_ratio": mediaGeminiAspectRatio,
	}
	if id != "gemini-2.5-flash-image" {
		parameters["resolution"] = mediaGeminiResolution
	}
	return parameters
}

func mediaPresetProfile(id string, endpoints map[string]bool, generation *jsplugin.RoutingGeneration) (model.MediaModelProfile, bool) {
	if mediaPresetGeminiImageIDs[id] && endpoints[string(constant.EndpointTypeGemini)] {
		parameters := mediaGeminiParameters(id)
		return model.MediaModelProfile{
			ID:   id,
			Type: "image",
			Operations: map[string]model.MediaOperation{
				"text_to_image": {
					Protocol:   "gemini_generate_content",
					Path:       "/v1beta/models/{model}:generateContent",
					Parameters: parameters,
					Reference:  mediaNoReference(),
				},
				"image_to_image": {
					Protocol:   "gemini_generate_content",
					Path:       "/v1beta/models/{model}:generateContent",
					Parameters: parameters,
					Reference:  mediaInlineReference(),
				},
			},
		}, true
	}
	if mediaPresetOpenAIImageIDs[id] && endpoints[string(constant.EndpointTypeImageGeneration)] {
		return model.MediaModelProfile{
			ID:   id,
			Type: "image",
			Operations: map[string]model.MediaOperation{
				"text_to_image": {
					Protocol:   "openai_images",
					Path:       "/v1/images/generations",
					Parameters: map[string]model.MediaParameter{},
					Reference:  mediaNoReference(),
				},
				"image_to_image": {
					Protocol:   "openai_images",
					Path:       "/v1/images/edits",
					Parameters: map[string]model.MediaParameter{},
					Reference:  mediaInlineReference(),
				},
			},
		}, true
	}
	hasImage := endpoints[string(constant.EndpointTypeImageGeneration)]
	hasVideo := endpoints[string(constant.EndpointTypeOpenAIVideo)]
	if hasVideo && (!hasImage || len(mediaVideoBindingList(generation, id)) > 0) {
		return model.MediaModelProfile{
			ID:   id,
			Type: "video",
			Operations: map[string]model.MediaOperation{
				"text_to_video": {
					Protocol:   "openai_video",
					Path:       "/v1/videos",
					Parameters: map[string]model.MediaParameter{},
					Reference:  mediaNoReference(),
				},
			},
		}, true
	}
	if hasImage {
		return model.MediaModelProfile{
			ID:   id,
			Type: "image",
			Operations: map[string]model.MediaOperation{
				"text_to_image": {
					Protocol:   "openai_images",
					Path:       "/v1/images/generations",
					Parameters: map[string]model.MediaParameter{},
					Reference:  mediaNoReference(),
				},
			},
		}, true
	}
	return model.MediaModelProfile{}, false
}

func mediaVideoBindingList(generation *jsplugin.RoutingGeneration, modelName string) []jsplugin.ProtocolBinding {
	_, bindings := mediaVideoBindings(generation, "openai_video", modelName)
	return bindings
}

func resolveMediaPolicy(policy model.MediaPolicy) (model.MediaPolicy, error) {
	ordinary, _, err := model.SearchModelsWithChannels("", "", "", "", 0, -1)
	if err != nil {
		return model.MediaPolicy{}, err
	}
	connections, err := model.GetModelConnections()
	if err != nil {
		return model.MediaPolicy{}, err
	}
	endpointsByID := make(map[string]map[string]bool)
	for _, connection := range connections {
		set := endpointsByID[connection.Model]
		if set == nil {
			set = make(map[string]bool)
			endpointsByID[connection.Model] = set
		}
		for _, endpoint := range common.GetEndpointTypesByChannelType(connection.ChannelType, connection.Model) {
			set[string(endpoint)] = true
		}
	}
	stored := make(map[string]model.MediaModelProfile, len(policy.Models))
	for _, profile := range policy.Models {
		stored[profile.ID] = profile
	}
	generation := jsplugin.DefaultRegistry.Generation()
	resolved := policy
	resolved.Models = make([]model.MediaModelProfile, 0)
	seen := make(map[string]bool)
	for _, row := range ordinary {
		if row == nil || row.NameRule != model.NameRuleExact || seen[row.ModelName] {
			continue
		}
		seen[row.ModelName] = true
		endpoints := endpointsByID[row.ModelName]
		if endpoints == nil {
			endpoints = make(map[string]bool)
		}
		for _, endpoint := range model.GetModelSupportEndpointTypes(row.ModelName) {
			endpoints[string(endpoint)] = true
		}
		if strings.TrimSpace(row.Endpoints) != "" {
			var declared map[string]json.RawMessage
			if err := common.UnmarshalJsonStr(row.Endpoints, &declared); err != nil {
				return model.MediaPolicy{}, fmt.Errorf("model %q endpoints metadata is malformed", row.ModelName)
			}
			for name, raw := range declared {
				kind := common.GetJsonType(raw)
				if kind == "string" || kind == "object" {
					endpoints[name] = true
				}
			}
		}
		if len(mediaVideoBindingList(generation, row.ModelName)) > 0 {
			endpoints[string(constant.EndpointTypeOpenAIVideo)] = true
		}
		profile, ok := mediaPresetProfile(row.ModelName, endpoints, generation)
		previous, existed := stored[row.ModelName]
		if !ok && existed {
			profile = previous
			ok = true
		}
		if !ok {
			continue
		}
		if existed {
			profile.SelectionHint = previous.SelectionHint
		}
		resolved.Models = append(resolved.Models, profile)
	}
	slices.SortFunc(resolved.Models, func(a, b model.MediaModelProfile) int {
		return strings.Compare(a.ID, b.ID)
	})
	return resolved, nil
}

func mediaCatalogUnavailable(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"code":    "media_catalog_unavailable",
		"message": "media model catalog is unavailable",
	})
}

func GetMediaModelsPolicy(c *gin.Context) {
	setNoStore(c)
	snapshot, err := model.GetMediaPolicy()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    "media_policy_unavailable",
			"message": "media model policy is unavailable",
		})
		return
	}
	resolved, err := resolveMediaPolicy(snapshot.Policy)
	if err != nil {
		mediaCatalogUnavailable(c)
		return
	}
	snapshot.Policy = resolved
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    snapshot,
	})
}

func UpdateMediaModelsPolicy(c *gin.Context) {
	setNoStore(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, mediaPolicyMaxBodyBytes)
	var request mediaPolicyUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		mediaPolicyValidationError(c, "request body must be valid JSON within 1MiB")
		return
	}
	if strings.TrimSpace(request.ExpectedVersion) == "" {
		mediaPolicyValidationError(c, "expected_version is required")
		return
	}
	if len(request.Policy) == 0 {
		mediaPolicyValidationError(c, "policy is required")
		return
	}
	if err := model.ValidateMediaPolicyRaw(request.Policy); err != nil {
		mediaPolicyValidationError(c, err.Error())
		return
	}
	var policy model.MediaPolicy
	if err := common.Unmarshal(request.Policy, &policy); err != nil {
		mediaPolicyValidationError(c, "policy must match the media model policy schema")
		return
	}
	if err := model.ValidateMediaPolicy(policy); err != nil {
		mediaPolicyValidationError(c, err.Error())
		return
	}
	current, err := model.GetMediaPolicy()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    "media_policy_unavailable",
			"message": "media model policy is unavailable",
		})
		return
	}
	resolved, err := resolveMediaPolicy(current.Policy)
	if err != nil {
		mediaCatalogUnavailable(c)
		return
	}
	incomingByID := make(map[string]model.MediaModelProfile, len(policy.Models))
	for _, incoming := range policy.Models {
		known := false
		for i := range resolved.Models {
			if resolved.Models[i].ID == incoming.ID {
				known = true
				resolved.Models[i].SelectionHint = incoming.SelectionHint
				break
			}
		}
		if !known {
			incomingByID[incoming.ID] = incoming
		}
	}
	for id := range incomingByID {
		mediaPolicyValidationError(c, fmt.Sprintf("model %q is not an available media model", id))
		return
	}
	resolved.ImagePriority = policy.ImagePriority
	resolved.VideoPriority = policy.VideoPriority
	if err := model.ValidateMediaPolicy(resolved); err != nil {
		mediaPolicyValidationError(c, err.Error())
		return
	}
	snapshot, err := model.UpdateMediaPolicy(request.ExpectedVersion, resolved)
	if errors.Is(err, model.ErrMediaPolicyConflict) {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"code":    "media_policy_conflict",
			"message": "media model policy changed; reload before saving",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    "media_policy_unavailable",
			"message": "media model policy could not be saved",
		})
		return
	}
	modelIDs := make([]string, 0, len(snapshot.Policy.Models))
	for _, profile := range snapshot.Policy.Models {
		modelIDs = append(modelIDs, profile.ID)
	}
	recordManageAudit(c, "model.media_policy.update", map[string]any{
		"config_version": snapshot.ConfigVersion,
		"model_ids":      modelIDs,
		"model_count":    len(modelIDs),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    snapshot,
	})
}

func mediaPolicyValidationError(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"code":    "invalid_media_policy",
		"message": message,
	})
}

func setNoStore(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
}

func MediaModels(c *gin.Context) {
	setNoStore(c)
	mediaType, operation, ok := parseMediaModelsQuery(c)
	if !ok {
		return
	}
	snapshot, err := model.GetMediaPolicy()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    "media_policy_unavailable",
			"message": "media model policy is unavailable",
		})
		return
	}
	resolved, err := resolveMediaPolicy(snapshot.Policy)
	if err != nil {
		mediaCatalogUnavailable(c)
		return
	}
	snapshot.Policy = resolved
	acceptUnset, err := mediaAcceptUnsetModels(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    "settings_lookup_failed",
			"message": "user settings lookup failed",
		})
		return
	}
	groups, err := getModelListGroups(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    "group_lookup_failed",
			"message": "get user group failed",
		})
		return
	}
	tokenLimited := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
	tokenModelLimit := map[string]bool{}
	if tokenLimited {
		if value, exists := common.GetContextKey(c, constant.ContextKeyTokenModelLimit); exists {
			tokenModelLimit, _ = value.(map[string]bool)
		}
	}
	generation := jsplugin.DefaultRegistry.Generation()
	constraints := service.GetChannelConstraints(c)
	items := make([]mediaCatalogItem, 0, len(snapshot.Policy.Models))
	for _, profile := range snapshot.Policy.Models {
		if mediaType != "" && profile.Type != mediaType {
			continue
		}
		if tokenLimited {
			matching := ratio_setting.RoutingMatchModelName(profile.ID)
			if !tokenModelLimit[profile.ID] && !tokenModelLimit[matching] {
				continue
			}
		}
		operations := make(map[string]model.MediaOperation, len(profile.Operations))
		var opErr error
		for operationName, op := range profile.Operations {
			if operation != "" && operationName != operation {
				continue
			}
			supported, err := mediaOperationAvailable(generation, constraints, groups.ownerGroups, profile, operationName, op, acceptUnset)
			if err != nil {
				opErr = err
				break
			}
			if supported {
				operations[operationName] = op
			}
		}
		if opErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"code":    "channel_lookup_failed",
				"message": "channel lookup failed",
			})
			return
		}
		if len(operations) == 0 {
			continue
		}
		items = append(items, mediaCatalogItem{
			ID:            profile.ID,
			Type:          profile.Type,
			Operations:    operations,
			SelectionHint: profile.SelectionHint,
			Availability: mediaAvailabilityWire{
				Routable: true,
				Health:   "unknown",
			},
		})
	}
	orderMediaCatalogItems(items, snapshot.Policy)
	defaults := mediaCatalogDefaults(items, snapshot.Policy, mediaType, operation)
	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"version":        1,
		"object":         "media_model_catalog",
		"generated_at":   time.Now().UTC().Format(time.RFC3339),
		"config_version": snapshot.ConfigVersion,
		"data":           items,
		"defaults":       defaults,
	})
}

func parseMediaModelsQuery(c *gin.Context) (mediaType string, operation string, ok bool) {
	types := c.Request.URL.Query()["type"]
	operations := c.Request.URL.Query()["operation"]
	if len(types) > 1 || len(operations) > 1 {
		mediaModelsQueryError(c, "type and operation accept a single value")
		return "", "", false
	}
	if len(types) == 1 {
		mediaType = types[0]
		if mediaType != "image" && mediaType != "video" {
			mediaModelsQueryError(c, "type must be image or video")
			return "", "", false
		}
	}
	if len(operations) == 1 {
		operation = operations[0]
		inferred, known := mediaOperationMediaType[operation]
		if !known {
			mediaModelsQueryError(c, "operation must be one of text_to_image, image_to_image, text_to_video, image_to_video")
			return "", "", false
		}
		if mediaType != "" && mediaType != inferred {
			mediaModelsQueryError(c, "operation does not match type")
			return "", "", false
		}
		mediaType = inferred
	}
	return mediaType, operation, true
}

func mediaModelsQueryError(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"code":    "invalid_media_models_query",
		"message": message,
	})
}

func mediaAcceptUnsetModels(c *gin.Context) (bool, error) {
	if operation_setting.SelfUseModeEnabled {
		return true, nil
	}
	userID := c.GetInt("id")
	if userID <= 0 {
		return false, nil
	}
	userSettings, err := model.GetUserSetting(userID, false)
	if err != nil {
		return false, err
	}
	return userSettings.AcceptUnsetRatioModel, nil
}

func mediaOperationAvailable(generation *jsplugin.RoutingGeneration, constraints *hostdto.ChannelConstraints, groups []string, profile model.MediaModelProfile, operationName string, op model.MediaOperation, acceptUnset bool) (bool, error) {
	if op.Protocol == "openai_video" && !jsplugin.DefaultRegistry.Enabled() {
		return false, nil
	}
	candidates, err := model.GetMediaModelChannels(groups, profile.ID, constraints)
	if err != nil {
		return false, err
	}
	opPath := op.Path
	if strings.Contains(opPath, "{model}") {
		opPath = strings.ReplaceAll(opPath, "{model}", url.PathEscape(profile.ID))
	}
	filters := make([]hostdto.ChannelFilter, 0, len(constraints.Filters)+1)
	filters = append(filters, constraints.Filters...)
	filters = append(filters, hostdto.ChannelFilter{Kind: hostdto.FilterRequestPath, RequestPath: opPath})
	mappedModel, bindings := mediaVideoBindings(generation, op.Protocol, profile.ID)
	if len(bindings) > 0 {
		identity := hostdto.ChannelFilter{Kind: hostdto.FilterTaskPluginIdentity}
		for _, binding := range bindings {
			if binding.Plugin == nil {
				continue
			}
			if identity.TaskPluginKey == "" {
				identity.TaskPluginKey = binding.Plugin.Meta.Key
			}
			identity.TaskPluginKeys = append(identity.TaskPluginKeys, binding.Plugin.Meta.Key)
			identity.TaskPluginChannelTypes = append(identity.TaskPluginChannelTypes, binding.Plugin.Meta.ChannelTypes...)
		}
		filters = append(filters, identity)
	}
	eligible := 0
	for _, channel := range candidates {
		if satisfied, _ := model.ChannelSatisfiesFilters(channel, profile.ID, filters); !satisfied {
			continue
		}
		boundPlugin, supported := mediaChannelSupportsOperation(generation, bindings, channel, profile, operationName, op)
		if !supported {
			return false, nil
		}
		if !mediaChannelBillingOK(boundPlugin, profile.ID, mappedModel, acceptUnset) {
			return false, nil
		}
		eligible++
	}
	return eligible > 0, nil
}

func mediaVideoBindings(generation *jsplugin.RoutingGeneration, protocol string, modelName string) (string, []jsplugin.ProtocolBinding) {
	if protocol != "openai_video" || generation == nil {
		return modelName, nil
	}
	lookup := modelName
	if canonical, ok := generation.CanonicalModel(modelName); ok && canonical != "" {
		lookup = canonical
	}
	bindings := generation.LookupEndpointCandidates(http.MethodPost, "/v1/videos", lookup)
	if len(bindings) == 0 {
		if target, resolved := model.ResolveTaskModelAlias(generation, modelName); resolved && target.Declared != "" {
			if aliasBindings := generation.LookupEndpointCandidates(http.MethodPost, "/v1/videos", target.Declared); len(aliasBindings) > 0 {
				return target.Declared, aliasBindings
			}
		}
	}
	return lookup, bindings
}

func mediaChannelSupportsOperation(generation *jsplugin.RoutingGeneration, bindings []jsplugin.ProtocolBinding, channel *model.Channel, profile model.MediaModelProfile, operationName string, op model.MediaOperation) (*jsplugin.LoadedPlugin, bool) {
	switch op.Protocol {
	case "openai_images":
		switch channel.Type {
		case constant.ChannelTypeOpenAI, constant.ChannelTypeAzure, constant.ChannelTypeNewAPI:
			return nil, true
		case constant.ChannelTypeAdvancedCustom:
			return nil, true
		case constant.ChannelTypeGemini:
			if operationName != "text_to_image" {
				return nil, false
			}
			mapped := model.MappedUpstreamModel(channel, profile.ID)
			return nil, strings.HasPrefix(mapped, "imagen")
		case constant.ChannelTypeXai:
			if operationName != "text_to_image" {
				return nil, false
			}
			for param := range op.Parameters {
				if param != "n" && param != "response_format" {
					return nil, false
				}
			}
			return nil, true
		default:
			return nil, false
		}
	case "gemini_generate_content":
		switch channel.Type {
		case constant.ChannelTypeGemini, constant.ChannelTypeNewAPI, constant.ChannelTypeAdvancedCustom:
			return nil, true
		default:
			return nil, false
		}
	case "openai_video":
		return mediaVideoChannelSupport(generation, bindings, channel)
	default:
		return nil, false
	}
}

func mediaVideoChannelSupport(generation *jsplugin.RoutingGeneration, bindings []jsplugin.ProtocolBinding, channel *model.Channel) (*jsplugin.LoadedPlugin, bool) {
	if channel.Type == constant.ChannelTypeTaskPlugin {
		key := channel.GetSetting().TaskPluginKey
		for _, binding := range bindings {
			if binding.Plugin != nil && binding.Plugin.Meta.Key == key {
				return binding.Plugin, true
			}
		}
		return nil, false
	}
	for _, binding := range bindings {
		if binding.Plugin != nil && slices.Contains(binding.Plugin.Meta.ChannelTypes, channel.Type) {
			return binding.Plugin, true
		}
	}
	if len(bindings) == 0 && generation != nil && (channel.Type == constant.ChannelTypeOpenAI || channel.Type == constant.ChannelTypeSora) {
		if plugin, ok := generation.GetByChannelType(channel.Type); ok && plugin != nil && pluginDeclaresOpenAIVideo(plugin) {
			return plugin, true
		}
	}
	return nil, false
}

func pluginDeclaresOpenAIVideo(plugin *jsplugin.LoadedPlugin) bool {
	for _, claim := range plugin.Meta.Protocols {
		if claim.Name == "openai_video" {
			return true
		}
	}
	return false
}

func mediaChannelBillingOK(plugin *jsplugin.LoadedPlugin, modelName string, mappedModel string, acceptUnset bool) bool {
	if plugin != nil {
		expr, exists := billing_setting.ResolveTaskBillingExpr(plugin.Meta.Key, modelName, mappedModel)
		if exists || billing_setting.GetBillingMode(modelName) == billing_setting.BillingModeTieredExpr {
			schemaModel := mappedModel
			if schemaModel == "" {
				schemaModel = modelName
			}
			schema, _ := plugin.Meta.UsageForModel(schemaModel)
			return billing_setting.TaskExprCompatible(expr, schema)
		}
	}
	if acceptUnset {
		return true
	}
	return helper.HasModelBillingConfig(modelName)
}

func orderMediaCatalogItems(items []mediaCatalogItem, policy model.MediaPolicy) {
	rank := make(map[string]int)
	for i, id := range policy.ImagePriority {
		rank["image:"+id] = i
	}
	for i, id := range policy.VideoPriority {
		rank["video:"+id] = i
	}
	slices.SortStableFunc(items, func(a, b mediaCatalogItem) int {
		rankA, okA := rank[a.Type+":"+a.ID]
		rankB, okB := rank[b.Type+":"+b.ID]
		switch {
		case okA && !okB:
			return -1
		case !okA && okB:
			return 1
		case okA && okB && rankA != rankB:
			return rankA - rankB
		}
		if a.Type != b.Type {
			return strings.Compare(a.Type, b.Type)
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func mediaCatalogDefaults(items []mediaCatalogItem, policy model.MediaPolicy, mediaType string, operation string) map[string]mediaDefaultEntry {
	result := make(map[string]mediaDefaultEntry, 2)
	for _, kind := range []string{"image", "video"} {
		if mediaType != "" && mediaType != kind {
			result[kind] = mediaDefaultEntry{ReasonCode: "not_requested"}
			continue
		}
		wanted := operation
		if wanted == "" {
			wanted = mediaDefaultOperation[kind]
		}
		priorities := policy.ImagePriority
		if kind == "video" {
			priorities = policy.VideoPriority
		}
		hasCandidates := false
		var chosen *string
		for _, id := range priorities {
			for i := range items {
				if items[i].Type != kind || items[i].ID != id {
					continue
				}
				hasCandidates = true
				if _, exists := items[i].Operations[wanted]; exists && chosen == nil {
					modelID := items[i].ID
					chosen = &modelID
				}
			}
			if chosen != nil {
				break
			}
		}
		if !hasCandidates {
			for i := range items {
				if items[i].Type == kind {
					hasCandidates = true
					break
				}
			}
		}
		switch {
		case chosen != nil:
			result[kind] = mediaDefaultEntry{Model: chosen, ReasonCode: "ok"}
		case !hasCandidates:
			result[kind] = mediaDefaultEntry{ReasonCode: "no_available_model"}
		default:
			result[kind] = mediaDefaultEntry{ReasonCode: "no_recommended_model"}
		}
	}
	return result
}
