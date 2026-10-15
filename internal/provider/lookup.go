package provider

import (
	"fmt"
	"strings"
)

// findByName returns the one item whose name matches exactly, or an error for zero or several matches.
func findByName[T any](kind, name string, items []T, nameOf func(T) string, idOf func(T) string) (T, error) {
	var found []T
	for _, it := range items {
		if nameOf(it) == name {
			found = append(found, it)
		}
	}
	var zero T
	switch len(found) {
	case 0:
		return zero, fmt.Errorf("no %s is named %q", kind, name)
	case 1:
		return found[0], nil
	}
	ids := make([]string, len(found))
	for i, it := range found {
		ids[i] = idOf(it)
	}
	return zero, fmt.Errorf("%d %ss are named %q (IDs %s); rename one or use its ID directly", len(found), kind, name, strings.Join(ids, ", "))
}
