// Package utils holds small helpers shared across storage-vendor code: the
// CSI-import resolvers (pkg/storage/resolver/...), the xcopy copy-offload
// populator (cmd/vsphere-copy-offload-populator/internal/...), and RDM
// storage-map resolution (pkg/controller/plan/adapter/vsphere).
package utils

import "strings"

const (
	// naaHexLen is the length in hex characters of an NAA-6 identifier (16 bytes).
	naaHexLen = 32
	// vmlHeaderLen is the length in hex characters of the fixed preamble ESXi 7.0+
	// prepends to the NAA payload in a "vml." runtime device name: naming-type(1B)
	// + device-type(1B) + LUN-ID(2B) + reserved(1B).
	// See: https://knowledge.broadcom.com/external/article/379673
	vmlHeaderLen = 10
)

// NAAHexFromDeviceName extracts the hex digits of a disk's NAA identifier from a
// vSphere device name containing a "naa." or "vml." marker. Accepts a canonical
// NAA name ("naa.<hex>"), a full device path ("/vmfs/devices/disks/naa.<hex>"), or
// an ESXi "vml.<hex>" runtime name, where the NAA payload follows a fixed-length
// preamble (ESXi 7.0+). The vml payload is capped at 32 hex chars to exclude the
// trailing per-LUN hash vSphere appends after it.
//
// Returns ok=false if deviceName contains neither marker, or a "vml." name has no
// payload left after the preamble.
func NAAHexFromDeviceName(deviceName string) (hex string, ok bool) {
	lower := strings.ToLower(strings.TrimSpace(deviceName))

	if idx := strings.LastIndex(lower, "naa."); idx >= 0 {
		return lower[idx+len("naa."):], true
	}

	if idx := strings.LastIndex(lower, "vml."); idx >= 0 {
		rest := lower[idx+len("vml."):]
		if len(rest) <= vmlHeaderLen {
			return "", false
		}
		payload := rest[vmlHeaderLen:]
		if len(payload) > naaHexLen {
			payload = payload[:naaHexLen]
		}
		return payload, true
	}

	return "", false
}
