/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

    10|Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nimparser

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// NormalizeLegacyManifest converts a legacy wrapped manifest (model_profile + manifest list) into a v1 hash-keyed profile map YAML.
// Returns (normalizedYAML, true, nil) when the input is legacy and conversion succeeds.
// Returns (nil, false, nil) when the input is not a legacy wrapped manifest.
// Returns a non-nil error when the input looks legacy but cannot be normalized.
func NormalizeLegacyManifest(data []byte) ([]byte, bool, error) {
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		// Not a mapping document; leave for the standard parsers.
		return nil, false, nil
	}

	modelProfile, hasModelProfile := root["model_profile"].(string)
	if !hasModelProfile || modelProfile == "" {
		return nil, false, nil
	}

	manifestRaw, hasManifest := root["manifest"]
	if !hasManifest {
		return nil, false, nil
	}

	// schema_version documents are handled by the v2 parser.
	if _, hasSchemaVersion := root["schema_version"]; hasSchemaVersion {
		return nil, false, nil
	}

	manifestList, ok := manifestRaw.([]interface{})
	if !ok {
		return nil, true, fmt.Errorf("legacy model manifest: top-level 'manifest' must be a list")
	}
	if len(manifestList) == 0 {
		return nil, true, fmt.Errorf("legacy model manifest: top-level 'manifest' list is empty")
	}

	normalized := make(map[string]interface{}, len(manifestList))
	for i, item := range manifestList {
		entry, ok := asStringKeyMap(item)
		if !ok {
			return nil, true, fmt.Errorf("legacy model manifest: entry %d is not a mapping", i)
		}

		id, profile, err := extractLegacyProfile(entry, modelProfile)
		if err != nil {
			return nil, true, fmt.Errorf("legacy model manifest: entry %d: %w", i, err)
		}
		if _, exists := normalized[id]; exists {
			return nil, true, fmt.Errorf("legacy model manifest: duplicate profile id %q", id)
		}
		normalized[id] = profile
	}

	out, err := yaml.Marshal(normalized)
	if err != nil {
		return nil, true, fmt.Errorf("legacy model manifest: failed to marshal normalized v1 manifest: %w", err)
	}
	return out, true, nil
}

// extractLegacyProfile pulls a profile ID and profile fields from one legacy manifest list entry (id-based or single-key hash form).
func extractLegacyProfile(entry map[string]interface{}, modelProfile string) (string, map[string]interface{}, error) {
	if id, ok := entry["id"].(string); ok && id != "" {
		profile := copyStringKeyMap(entry)
		delete(profile, "id")
		return id, coerceLegacyProfile(profile, modelProfile), nil
	}

	// Single-key form: "<profile-hash>": { ...profile fields... }
	if len(entry) == 1 {
		for id, rawProfile := range entry {
			profile, ok := asStringKeyMap(rawProfile)
			if !ok {
				return "", nil, fmt.Errorf("single-key profile %q is not a mapping", id)
			}
			if id == "" {
				return "", nil, fmt.Errorf("profile hash key is empty")
			}
			return id, coerceLegacyProfile(copyStringKeyMap(profile), modelProfile), nil
		}
	}

	return "", nil, fmt.Errorf("profile entry must include an 'id' field or be a single-key hash map")
}

// coerceLegacyProfile fills missing model from model_profile and stringifies tag values for v1 compatibility.
func coerceLegacyProfile(profile map[string]interface{}, modelProfile string) map[string]interface{} {
	if model, ok := profile["model"].(string); !ok || model == "" {
		profile["model"] = modelProfile
	}

	if tagsRaw, ok := profile["tags"]; ok {
		if tags, ok := asStringKeyMap(tagsRaw); ok {
			stringTags := make(map[string]string, len(tags))
			for k, v := range tags {
				stringTags[k] = fmt.Sprint(v)
			}
			profile["tags"] = stringTags
		}
	}

	return profile
}

// asStringKeyMap converts a value to map[string]interface{} when it is a string-keyed or interface-keyed map.
func asStringKeyMap(v interface{}) (map[string]interface{}, bool) {
	switch m := v.(type) {
	case map[string]interface{}:
		return m, true
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, val := range m {
			ks, ok := k.(string)
			if !ok {
				return nil, false
			}
			out[ks] = val
		}
		return out, true
	default:
		return nil, false
	}
}

// copyStringKeyMap returns a shallow copy of a string-keyed map so callers can mutate safely.
func copyStringKeyMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
