package pull

// UpdateOptions controls scheduling independently of completion deduplication.
// The zero value has the same behavior as Update().
type UpdateOptions struct {
	// AllowEarly permits execution before today's close boundary, certifying only
	// the preceding window. It does not bypass completion deduplication.
	AllowEarly bool
	// Recheck ignores the selected window's completion marker. It still performs
	// normal incremental fetching, retaining historical coverage and empty caches.
	// It does not permit early execution unless AllowEarly is also set.
	Recheck bool
}
