package resolver

// DiskType classifies the vSphere backing type for a VM disk.
type DiskType string

const (
	DiskTypeVVol DiskType = "vvol"
	DiskTypeRDM  DiskType = "rdm"
	DiskTypeVMDK DiskType = "vmdk"
)

// DiskBacking contains disk backing information as returned by govmomi.
// Shape intentionally matches the populator's internal/vmware.DiskBacking — shared to avoid duplication.
type DiskBacking struct {
	// VVolId is non-empty when the disk is VVol-backed (govmomi BackingObjectId).
	VVolId string
	// IsRDM is true when the disk is a Raw Device Mapping.
	IsRDM bool
	// DeviceName is the underlying device path or VMDK file name.
	DeviceName string
}

// Classify returns the DiskType for this backing.
func (b *DiskBacking) Classify() DiskType {
	switch {
	case b.VVolId != "":
		return DiskTypeVVol
	case b.IsRDM:
		return DiskTypeRDM
	default:
		return DiskTypeVMDK
	}
}
