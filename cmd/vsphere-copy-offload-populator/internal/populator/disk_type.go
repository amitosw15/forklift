package populator

// populatorSettings controls which optimized methods are disabled.
// All methods are enabled by default unless explicitly disabled.
// VMDK/Xcopy cannot be disabled as it's the default fallback.
type populatorSettings struct {
	// VVolDisabled disables VVol optimization when disk is VVol-backed
	VVolDisabled bool
	// RDMDisabled disables RDM optimization when disk is RDM-backed
	RDMDisabled bool
	// Note: VMDK cannot be disabled as it's the default fallback
}
