package source

import "regexp"

// Dicom study UID are dot-separated numeric components (PS3.5 §9.1).
var uidPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)

func ValidUID(s string) bool {
	return s != "" && len(s) <= 64 && uidPattern.MatchString(s)
}
