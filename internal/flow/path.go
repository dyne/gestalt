package flow

import (
	"fmt"
	"strings"
)

const managedFlowSuffix = ".flow.yaml"

func normalizeFlowID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(id))
	lastDash := false
	for _, r := range id {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if allowed {
			if r == '-' {
				if lastDash {
					continue
				}
				lastDash = true
			} else {
				lastDash = false
			}
			b.WriteRune(r)
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func managedFlowFilename(id string) (string, error) {
	normalized := normalizeFlowID(id)
	if normalized == "" {
		return "", &ValidationError{Kind: ValidationBadRequest, Message: "trigger id must normalize to a non-empty filename"}
	}
	return normalized + managedFlowSuffix, nil
}

func validateManagedFilenameCollisions(triggers []EventTrigger) error {
	seen := map[string]string{}
	for _, trigger := range triggers {
		filename, err := managedFlowFilename(trigger.ID)
		if err != nil {
			return err
		}
		if existing, ok := seen[filename]; ok {
			if existing != trigger.ID {
				return &ValidationError{
					Kind:    ValidationConflict,
					Message: fmt.Sprintf("trigger ids %q and %q map to the same managed filename %q", existing, trigger.ID, filename),
				}
			}
			continue
		}
		seen[filename] = trigger.ID
	}
	return nil
}
