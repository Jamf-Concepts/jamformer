// Copyright 2026, Jamf Software LLC

package multienv

// MatchResources matches resources across environments by (resource_type, label).
// Labels are derived from Jamf object names via naming.Sanitize(), so instances
// with the same resource names produce matching labels.
func MatchResources(envResults map[string]*PerEnvResult, envNames []string) []MatchedResource {
	type matchKey struct {
		resourceType string
		label        string
	}
	type entry struct {
		name string
		ids  map[string]string
	}

	lookup := make(map[matchKey]*entry)

	for _, envName := range envNames {
		result := envResults[envName]
		if result == nil {
			continue
		}
		for _, r := range result.Resources {
			key := matchKey{resourceType: r.TFType, label: r.Label}
			e, ok := lookup[key]
			if !ok {
				e = &entry{
					name: r.Name,
					ids:  make(map[string]string),
				}
				lookup[key] = e
			}
			// JamfID may be empty (e.g. a singleton with no recoverable import
			// ID); record it anyway so the resource is matched and placed, but
			// generateEnvImports will skip an empty ID.
			e.ids[envName] = r.JamfID
		}
	}

	numEnvs := len(envNames)
	var matches []MatchedResource
	for key, e := range lookup {
		present := make([]string, 0, len(e.ids))
		for _, name := range envNames {
			if _, ok := e.ids[name]; ok {
				present = append(present, name)
			}
		}
		matches = append(matches, MatchedResource{
			ResourceType: key.resourceType,
			Label:        key.label,
			Name:         e.name,
			IDs:          e.ids,
			Present:      present,
			AllEnvs:      len(present) == numEnvs,
		})
	}

	return matches
}
