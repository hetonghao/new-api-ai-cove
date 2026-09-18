package model

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MediaModelsPolicyOption = "MediaModelsPolicy"

var ErrMediaPolicyConflict = errors.New("media model policy changed; reload before saving")

const (
	mediaPolicyVersion1      = 1
	mediaPolicyMaxEntries    = 500
	mediaPolicyMaxIDBytes    = 128
	mediaPolicyMaxEnumValues = 64
	mediaPolicyMaxEnumBytes  = 256
	mediaPolicyMaxRefImages  = 16
	mediaPolicyMaxHintRunes  = 2000
)

type MediaParameter struct {
	Type    string   `json:"type"`
	Enum    []string `json:"enum,omitempty"`
	Minimum *int     `json:"minimum,omitempty"`
	Maximum *int     `json:"maximum,omitempty"`
}

type MediaReference struct {
	Input     string `json:"input"`
	MaxImages int    `json:"max_images"`
}

type MediaOperation struct {
	Protocol   string                    `json:"protocol"`
	Path       string                    `json:"path"`
	Parameters map[string]MediaParameter `json:"parameters"`
	Reference  MediaReference            `json:"reference"`
}

type MediaModelProfile struct {
	ID            string                    `json:"id"`
	Type          string                    `json:"type"`
	Operations    map[string]MediaOperation `json:"operations"`
	SelectionHint string                    `json:"selection_hint,omitempty"`
}

type MediaPolicy struct {
	Version       int                 `json:"version"`
	Models        []MediaModelProfile `json:"models"`
	ImagePriority []string            `json:"image_priority"`
	VideoPriority []string            `json:"video_priority"`
}

type MediaPolicySnapshot struct {
	ConfigVersion string      `json:"config_version"`
	Policy        MediaPolicy `json:"policy"`
}

var mediaPolicyMutationMu sync.Mutex

var mediaOperationPaths = map[string]map[string]string{
	"openai_images": {
		"text_to_image":  "/v1/images/generations",
		"image_to_image": "/v1/images/edits",
	},
	"gemini_generate_content": {
		"text_to_image":  "/v1beta/models/{model}:generateContent",
		"image_to_image": "/v1beta/models/{model}:generateContent",
	},
	"openai_video": {
		"text_to_video":  "/v1/videos",
		"image_to_video": "/v1/videos",
	},
}

var mediaOperationsByType = map[string][]string{
	"image": {"text_to_image", "image_to_image"},
	"video": {"text_to_video", "image_to_video"},
}

var mediaParameterAllowlist = map[string]map[string]bool{
	"openai_images":           {"size": true, "quality": true, "n": true, "response_format": true},
	"gemini_generate_content": {"aspect_ratio": true, "resolution": true},
	"openai_video":            {"seconds": true, "size": true, "aspect_ratio": true, "resolution": true},
}

var mediaPolicyObjectKeys = map[string]bool{
	"version": true, "models": true, "image_priority": true, "video_priority": true,
}

var mediaProfileObjectKeys = map[string]bool{
	"id": true, "type": true, "operations": true, "selection_hint": true,
}

var mediaOperationObjectKeys = map[string]bool{
	"protocol": true, "path": true, "parameters": true, "reference": true,
}

var mediaParameterObjectKeys = map[string]bool{
	"type": true, "enum": true, "minimum": true, "maximum": true,
}

var mediaReferenceObjectKeys = map[string]bool{
	"input": true, "max_images": true,
}

func defaultMediaPolicy() MediaPolicy {
	return MediaPolicy{
		Version:       mediaPolicyVersion1,
		Models:        []MediaModelProfile{},
		ImagePriority: []string{},
		VideoPriority: []string{},
	}
}

func normalizeMediaPolicy(policy MediaPolicy) MediaPolicy {
	if policy.Models == nil {
		policy.Models = []MediaModelProfile{}
	}
	if policy.ImagePriority == nil {
		policy.ImagePriority = []string{}
	}
	if policy.VideoPriority == nil {
		policy.VideoPriority = []string{}
	}
	for i := range policy.Models {
		if policy.Models[i].Operations == nil {
			policy.Models[i].Operations = map[string]MediaOperation{}
		}
	}
	return policy
}

func mediaPolicyVersion(policy MediaPolicy) (string, error) {
	encoded, err := common.Marshal(normalizeMediaPolicy(policy))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

func decodeStoredMediaPolicy(raw string) (MediaPolicy, error) {
	if err := ValidateMediaPolicyRaw([]byte(raw)); err != nil {
		return MediaPolicy{}, err
	}
	var policy MediaPolicy
	if err := common.UnmarshalJsonStr(raw, &policy); err != nil {
		return MediaPolicy{}, fmt.Errorf("stored media model policy is malformed: %w", err)
	}
	if err := ValidateMediaPolicy(policy); err != nil {
		return MediaPolicy{}, fmt.Errorf("stored media model policy is invalid: %w", err)
	}
	return normalizeMediaPolicy(policy), nil
}

func GetMediaPolicy() (*MediaPolicySnapshot, error) {
	var row Option
	err := DB.Where(commonKeyCol+" = ?", MediaModelsPolicyOption).Take(&row).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		policy := defaultMediaPolicy()
		version, versionErr := mediaPolicyVersion(policy)
		if versionErr != nil {
			return nil, versionErr
		}
		return &MediaPolicySnapshot{ConfigVersion: version, Policy: policy}, nil
	}
	policy, err := decodeStoredMediaPolicy(row.Value)
	if err != nil {
		return nil, err
	}
	version, err := mediaPolicyVersion(policy)
	if err != nil {
		return nil, err
	}
	return &MediaPolicySnapshot{ConfigVersion: version, Policy: policy}, nil
}

func UpdateMediaPolicy(expectedVersion string, policy MediaPolicy) (*MediaPolicySnapshot, error) {
	if strings.TrimSpace(expectedVersion) == "" {
		return nil, ErrMediaPolicyConflict
	}
	if err := ValidateMediaPolicy(policy); err != nil {
		return nil, err
	}
	policy = normalizeMediaPolicy(policy)
	encoded, err := common.Marshal(policy)
	if err != nil {
		return nil, err
	}
	mediaPolicyMutationMu.Lock()
	defer mediaPolicyMutationMu.Unlock()
	var committedVersion string
	err = DB.Transaction(func(tx *gorm.DB) error {
		defaultPolicy := defaultMediaPolicy()
		defaultEncoded, err := common.Marshal(defaultPolicy)
		if err != nil {
			return err
		}
		row := Option{Key: MediaModelsPolicyOption, Value: string(defaultEncoded)}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		var current Option
		if err := lockForUpdate(tx).Where(commonKeyCol+" = ?", MediaModelsPolicyOption).Take(&current).Error; err != nil {
			return err
		}
		stored, err := decodeStoredMediaPolicy(current.Value)
		if err != nil {
			return err
		}
		currentVersion, err := mediaPolicyVersion(stored)
		if err != nil {
			return err
		}
		if currentVersion != expectedVersion {
			return ErrMediaPolicyConflict
		}
		committedVersion, err = mediaPolicyVersion(policy)
		if err != nil {
			return err
		}
		return tx.Model(&Option{}).Where(commonKeyCol+" = ?", MediaModelsPolicyOption).Update("value", string(encoded)).Error
	})
	if err != nil {
		return nil, err
	}
	return &MediaPolicySnapshot{ConfigVersion: committedVersion, Policy: policy}, nil
}

func ValidateMediaPolicy(policy MediaPolicy) error {
	if policy.Version != mediaPolicyVersion1 {
		return fmt.Errorf("version must be %d", mediaPolicyVersion1)
	}
	if policy.Models == nil || policy.ImagePriority == nil || policy.VideoPriority == nil {
		return errors.New("models, image_priority and video_priority are required arrays")
	}
	if len(policy.Models) > mediaPolicyMaxEntries || len(policy.ImagePriority) > mediaPolicyMaxEntries || len(policy.VideoPriority) > mediaPolicyMaxEntries {
		return fmt.Errorf("policy allows at most %d models and %d priority ids per type", mediaPolicyMaxEntries, mediaPolicyMaxEntries)
	}
	profiles := make(map[string]MediaModelProfile, len(policy.Models))
	for i := range policy.Models {
		profile := policy.Models[i]
		if err := validateMediaModelID(profile.ID); err != nil {
			return err
		}
		if _, duplicated := profiles[profile.ID]; duplicated {
			return fmt.Errorf("duplicate model id %q", profile.ID)
		}
		if utf8.RuneCountInString(profile.SelectionHint) > mediaPolicyMaxHintRunes {
			return fmt.Errorf("model %q selection_hint exceeds %d characters", profile.ID, mediaPolicyMaxHintRunes)
		}
		if profile.Type != "image" && profile.Type != "video" {
			return fmt.Errorf("model %q has unsupported type %q", profile.ID, profile.Type)
		}
		if len(profile.Operations) == 0 {
			return fmt.Errorf("model %q must declare at least one operation", profile.ID)
		}
		for operationName, operation := range profile.Operations {
			if err := validateMediaOperation(profile, operationName, operation); err != nil {
				return err
			}
		}
		profiles[profile.ID] = profile
	}
	for _, field := range []struct {
		name     string
		priority []string
		wantType string
	}{
		{"image_priority", policy.ImagePriority, "image"},
		{"video_priority", policy.VideoPriority, "video"},
	} {
		seen := make(map[string]struct{}, len(field.priority))
		for _, id := range field.priority {
			if err := validateMediaModelID(id); err != nil {
				return fmt.Errorf("%s: %w", field.name, err)
			}
			if _, duplicated := seen[id]; duplicated {
				return fmt.Errorf("%s contains duplicate model id %q", field.name, id)
			}
			seen[id] = struct{}{}
			profile, exists := profiles[id]
			if !exists {
				return fmt.Errorf("%s references unregistered model id %q", field.name, id)
			}
			if profile.Type != field.wantType {
				return fmt.Errorf("%s references %s model %q", field.name, profile.Type, id)
			}
		}
	}
	return nil
}

func validateMediaModelID(id string) error {
	if id == "" || strings.TrimSpace(id) != id {
		return errors.New("model id must be a non-empty trimmed string")
	}
	if len(id) > mediaPolicyMaxIDBytes {
		return fmt.Errorf("model id %q exceeds %d bytes", id, mediaPolicyMaxIDBytes)
	}
	for _, r := range id {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("model id %q contains whitespace or control characters", id)
		}
	}
	if strings.ContainsAny(id, "?#%\\") {
		return fmt.Errorf("model id %q contains reserved characters", id)
	}
	return nil
}

func validateMediaOperation(profile MediaModelProfile, name string, operation MediaOperation) error {
	typeOperations, knownType := mediaOperationsByType[profile.Type]
	if !knownType {
		return fmt.Errorf("model %q has unsupported type %q", profile.ID, profile.Type)
	}
	validOperation := false
	for _, allowed := range typeOperations {
		if name == allowed {
			validOperation = true
			break
		}
	}
	if !validOperation {
		return fmt.Errorf("model %q has operation %q that does not match type %q", profile.ID, name, profile.Type)
	}
	paths, knownProtocol := mediaOperationPaths[operation.Protocol]
	if !knownProtocol {
		return fmt.Errorf("model %q operation %q uses unsupported protocol %q", profile.ID, name, operation.Protocol)
	}
	expectedPath, supported := paths[name]
	if !supported || operation.Path == "" || operation.Path != expectedPath {
		return fmt.Errorf("model %q operation %q has an incompatible protocol or path", profile.ID, name)
	}
	if operation.Parameters == nil {
		return fmt.Errorf("model %q operation %q requires a parameters object", profile.ID, name)
	}
	for paramName, parameter := range operation.Parameters {
		if !mediaParameterAllowlist[operation.Protocol][paramName] {
			return fmt.Errorf("model %q operation %q does not allow parameter %q for protocol %q", profile.ID, name, paramName, operation.Protocol)
		}
		if err := validateMediaParameter(profile.ID, name, paramName, parameter); err != nil {
			return err
		}
	}
	return validateMediaReference(profile, name, operation)
}

func validateMediaParameter(modelID, operationName, name string, parameter MediaParameter) error {
	integer := name == "n" || name == "seconds"
	if (integer && parameter.Type != "integer") || (!integer && parameter.Type != "string") {
		return fmt.Errorf("model %q operation %q parameter %q has an incompatible type", modelID, operationName, name)
	}
	switch parameter.Type {
	case "string":
		if parameter.Minimum != nil || parameter.Maximum != nil {
			return fmt.Errorf("model %q operation %q parameter %q must not declare integer bounds", modelID, operationName, name)
		}
		if parameter.Enum != nil {
			if len(parameter.Enum) == 0 || len(parameter.Enum) > mediaPolicyMaxEnumValues {
				return fmt.Errorf("model %q operation %q parameter %q enum size is out of range", modelID, operationName, name)
			}
			seen := make(map[string]struct{}, len(parameter.Enum))
			for _, value := range parameter.Enum {
				if value == "" || strings.TrimSpace(value) != value {
					return fmt.Errorf("model %q operation %q parameter %q enum values must be non-empty trimmed strings", modelID, operationName, name)
				}
				if len(value) > mediaPolicyMaxEnumBytes {
					return fmt.Errorf("model %q operation %q parameter %q enum value exceeds %d bytes", modelID, operationName, name, mediaPolicyMaxEnumBytes)
				}
				if _, duplicated := seen[value]; duplicated {
					return fmt.Errorf("model %q operation %q parameter %q has duplicate enum value %q", modelID, operationName, name, value)
				}
				seen[value] = struct{}{}
			}
		}
	case "integer":
		if parameter.Enum != nil {
			return fmt.Errorf("model %q operation %q parameter %q must not declare an enum", modelID, operationName, name)
		}
		if parameter.Minimum == nil || parameter.Maximum == nil {
			return fmt.Errorf("model %q operation %q parameter %q requires explicit minimum and maximum", modelID, operationName, name)
		}
		if *parameter.Minimum < 1 || *parameter.Minimum > *parameter.Maximum {
			return fmt.Errorf("model %q operation %q parameter %q has invalid bounds", modelID, operationName, name)
		}
		switch name {
		case "n":
			if *parameter.Maximum > kitdto.MaxImageN {
				return fmt.Errorf("model %q operation %q parameter n maximum exceeds %d", modelID, operationName, kitdto.MaxImageN)
			}
		case "seconds":
			if *parameter.Maximum > relaycommon.MaxTaskDurationSeconds {
				return fmt.Errorf("model %q operation %q parameter seconds maximum exceeds %d", modelID, operationName, relaycommon.MaxTaskDurationSeconds)
			}
		default:
			return fmt.Errorf("model %q operation %q has unsupported integer parameter %q", modelID, operationName, name)
		}
	default:
		return fmt.Errorf("model %q operation %q parameter %q has unsupported type %q", modelID, operationName, name, parameter.Type)
	}
	return nil
}

func validateMediaReference(profile MediaModelProfile, name string, operation MediaOperation) error {
	reference := operation.Reference
	textOperation := name == "text_to_image" || name == "text_to_video"
	if textOperation {
		if reference.Input != "none" || reference.MaxImages != 0 {
			return fmt.Errorf("model %q operation %q must declare reference input none with max_images 0", profile.ID, name)
		}
		return nil
	}
	if reference.Input != "inline" && reference.Input != "url" {
		return fmt.Errorf("model %q operation %q requires reference input inline or url", profile.ID, name)
	}
	if reference.MaxImages < 1 || reference.MaxImages > mediaPolicyMaxRefImages {
		return fmt.Errorf("model %q operation %q reference max_images must be between 1 and %d", profile.ID, name, mediaPolicyMaxRefImages)
	}
	if profile.Type == "video" && reference.MaxImages != 1 {
		return fmt.Errorf("model %q operation %q accepts exactly one reference image", profile.ID, name)
	}
	if operation.Protocol == "gemini_generate_content" && reference.Input != "inline" {
		return fmt.Errorf("model %q operation %q requires inline references for protocol %q", profile.ID, name, operation.Protocol)
	}
	return nil
}

func ValidateMediaPolicyRaw(raw json.RawMessage) error {
	var document map[string]json.RawMessage
	if err := common.Unmarshal(raw, &document); err != nil {
		return errors.New("policy must be a JSON object")
	}
	for key := range document {
		if !mediaPolicyObjectKeys[key] {
			return fmt.Errorf("policy field %q is not supported", key)
		}
	}
	var profiles []json.RawMessage
	if modelsRaw, exists := document["models"]; exists {
		if err := common.Unmarshal(modelsRaw, &profiles); err != nil {
			return errors.New("policy models must be an array")
		}
	}
	for i, profileRaw := range profiles {
		var profile map[string]json.RawMessage
		if err := common.Unmarshal(profileRaw, &profile); err != nil {
			return fmt.Errorf("policy model %d must be an object", i)
		}
		for key := range profile {
			if !mediaProfileObjectKeys[key] {
				return fmt.Errorf("policy model %d field %q is not supported", i, key)
			}
		}
		if hint, exists := profile["selection_hint"]; exists && common.GetJsonType(hint) != "string" {
			return fmt.Errorf("policy model %d selection_hint must be a string", i)
		}
		var operations map[string]json.RawMessage
		if operationsRaw, exists := profile["operations"]; exists {
			if err := common.Unmarshal(operationsRaw, &operations); err != nil {
				return fmt.Errorf("policy model %d operations must be an object", i)
			}
		}
		for operationName, operationRaw := range operations {
			if err := validateMediaOperationRaw(i, operationName, operationRaw); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMediaOperationRaw(modelIndex int, name string, raw json.RawMessage) error {
	var operation map[string]json.RawMessage
	if err := common.Unmarshal(raw, &operation); err != nil {
		return fmt.Errorf("policy model %d operation %q must be an object", modelIndex, name)
	}
	for key := range operation {
		if !mediaOperationObjectKeys[key] {
			return fmt.Errorf("policy model %d operation %q field %q is not supported", modelIndex, name, key)
		}
	}
	var parameters map[string]json.RawMessage
	if parametersRaw, exists := operation["parameters"]; exists {
		if err := common.Unmarshal(parametersRaw, &parameters); err != nil {
			return fmt.Errorf("policy model %d operation %q parameters must be an object", modelIndex, name)
		}
	}
	for paramName, paramRaw := range parameters {
		var parameter map[string]json.RawMessage
		if err := common.Unmarshal(paramRaw, &parameter); err != nil {
			return fmt.Errorf("policy model %d operation %q parameter %q must be an object", modelIndex, name, paramName)
		}
		for key := range parameter {
			if !mediaParameterObjectKeys[key] {
				return fmt.Errorf("policy model %d operation %q parameter %q field %q is not supported", modelIndex, name, paramName, key)
			}
		}
	}
	if referenceRaw, exists := operation["reference"]; exists {
		var reference map[string]json.RawMessage
		if err := common.Unmarshal(referenceRaw, &reference); err != nil {
			return fmt.Errorf("policy model %d operation %q reference must be an object", modelIndex, name)
		}
		for key := range reference {
			if !mediaReferenceObjectKeys[key] {
				return fmt.Errorf("policy model %d operation %q reference field %q is not supported", modelIndex, name, key)
			}
		}
	}
	return nil
}
