package api

import "regexp"

// Leave space for artifact extensions and timestamped event filenames.
var pathID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)
