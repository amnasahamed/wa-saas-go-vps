package extraction

// MergeFields merges newly-extracted fields into the existing lead.Fields map.
//
// Rules ported from workflownew.json's Merge With Existing Lead1 node:
//   - Preserve existing nonempty values when new extraction is empty
//   - Lead ID is computed as "WA-{phone}" (kept stable across updates)
//   - For multi-value fields like subjects_failed, union the lists
//
// This is the canonical "don't overwrite real data with blanks" guard
// that the legacy system got wrong on Price Disclosed (overwriting "10000"
// with "No" when the next message didn't mention price).
func MergeFields(existing, extracted map[string]any) map[string]any {
	if existing == nil {
		existing = make(map[string]any)
	}
	if extracted == nil {
		return existing
	}

	for k, newVal := range extracted {
		oldVal, hasOld := existing[k]

		// Skip empty extracted values — preserve existing.
		if isEmpty(newVal) {
			continue
		}

		// Special handling for list fields — union them.
		if k == "subjects_failed" {
			merged := unionStringLists(oldVal, newVal)
			existing[k] = merged
			continue
		}

		// Otherwise overwrite with extracted value.
		_ = hasOld
		existing[k] = newVal
	}

	return existing
}

// isEmpty reports whether a value should be treated as "no information".
// Covers nil, empty string, empty slice, and zero numbers.
func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []string:
		return len(x) == 0
	case []any:
		return len(x) == 0
	case int:
		return x == 0
	case int64:
		return x == 0
	case float64:
		return x == 0
	default:
		return false
	}
}

// unionStringLists combines two list representations into a deduplicated []string.
func unionStringLists(oldVal, newVal any) []string {
	out := make(map[string]struct{})
	addAll := func(v any) {
		switch xs := v.(type) {
		case []string:
			for _, s := range xs {
				if s != "" {
					out[s] = struct{}{}
				}
			}
		case []any:
			for _, item := range xs {
				if s, ok := item.(string); ok && s != "" {
					out[s] = struct{}{}
				}
			}
		case string:
			// Comma-separated fallback
			if xs == "" {
				return
			}
			for _, p := range splitComma(xs) {
				if p != "" {
					out[p] = struct{}{}
				}
			}
		}
	}
	addAll(oldVal)
	addAll(newVal)
	result := make([]string, 0, len(out))
	for s := range out {
		result = append(result, s)
	}
	return result
}

// splitComma splits on comma and trims spaces.
func splitComma(s string) []string {
	parts := []string{}
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				parts = append(parts, trim(cur))
				cur = ""
			}
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		parts = append(parts, trim(cur))
	}
	return parts
}

func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// ComputeLeadID returns the canonical Lead ID for a phone: "WA-{phone}".
// Ported from workflownew.json's Upsert CRM Lead1 mapping.
func ComputeLeadID(phone string) string {
	return "WA-" + phone
}

// MapIntentToStatus maps GPT-4o's `intent` field to the canonical `lead_status`.
// Ported from workflownew.json's Map to CRM Fields1 node.
func MapIntentToStatus(intent string) string {
	switch intent {
	case "fee_inquiry":
		return "Interested"
	case "not_interested":
		return "Lost"
	case "enrolled":
		return "Enrolled"
	case "follow_up", "demo_request", "new_inquiry":
		return "Follow-up"
	default:
		return "Contacted"
	}
}
