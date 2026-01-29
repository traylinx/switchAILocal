package util

import "regexp"

var slugRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// IsValidPluginID checks if the plugin ID is a valid slug.
func IsValidPluginID(id string) bool {
	return slugRegex.MatchString(id)
}
