package utils

import "testing"

func TestNAAHexFromDeviceName(t *testing.T) {
	tests := []struct {
		name       string
		deviceName string
		wantHex    string
		wantOk     bool
	}{
		{
			name:       "naa. prefix",
			deviceName: "naa.624a93700123456789abcdef",
			wantHex:    "624a93700123456789abcdef",
			wantOk:     true,
		},
		{
			name:       "full device path with naa. prefix",
			deviceName: "/vmfs/devices/disks/naa.624a93700123456789abcdef",
			wantHex:    "624a93700123456789abcdef",
			wantOk:     true,
		},
		{
			name:       "uppercase naa. prefix is lowercased",
			deviceName: "NAA.624A93700123456789ABCDEF",
			wantHex:    "624a93700123456789abcdef",
			wantOk:     true,
		},
		{
			name:       "vml. prefix, ESXi 7.0+ format (MTV-6321 3PAR device)",
			deviceName: "vml.020002000060002ac0000000000000628200021f6b565620202020",
			wantHex:    "60002ac0000000000000628200021f6b",
			wantOk:     true,
		},
		{
			name:       "vml. prefix, real ONTAP RDM",
			deviceName: "vml.0200180000600a098038313954492458313032502d4c554e20432d",
			wantHex:    "600a098038313954492458313032502d",
			wantOk:     true,
		},
		{
			name:       "vml. prefix shorter than a full NAA-6 payload still returns available bytes",
			deviceName: "vml.0200010000624a93700abcdef012345678",
			wantHex:    "624a93700abcdef012345678",
			wantOk:     true,
		},
		{
			name:       "vml. prefix with no payload after preamble",
			deviceName: "vml.0200180000",
			wantHex:    "",
			wantOk:     false,
		},
		{
			name:       "naa. found after vml. marker takes precedence",
			deviceName: "vml.02000400006000097000022000285753303031313453594d4d4554naa.6000097000022000285753303031313454",
			wantHex:    "6000097000022000285753303031313454",
			wantOk:     true,
		},
		{
			name:       "no naa. or vml. marker",
			deviceName: "eui.0123456789abcdef",
			wantHex:    "",
			wantOk:     false,
		},
		{
			name:       "empty string",
			deviceName: "",
			wantHex:    "",
			wantOk:     false,
		},
		{
			name:       "full device path, longer serial than a synthetic 32-char example",
			deviceName: "/vmfs/devices/disks/naa.624a93700123456789abcdef0123456789",
			wantHex:    "624a93700123456789abcdef0123456789",
			wantOk:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hex, ok := NAAHexFromDeviceName(tt.deviceName)
			if ok != tt.wantOk {
				t.Fatalf("NAAHexFromDeviceName(%q) ok = %v, want %v", tt.deviceName, ok, tt.wantOk)
			}
			if hex != tt.wantHex {
				t.Errorf("NAAHexFromDeviceName(%q) = %q, want %q", tt.deviceName, hex, tt.wantHex)
			}
		})
	}
}

func TestNAAHexFromDeviceNameComparableAcrossFormats(t *testing.T) {
	naaHex, naaOk := NAAHexFromDeviceName("naa.6000097000022222000000000000001")
	vmlHex, vmlOk := NAAHexFromDeviceName("vml.02000400006000097000022222000000000000001")
	if !naaOk || !vmlOk {
		t.Fatalf("expected both extractions to succeed, got naaOk=%v vmlOk=%v", naaOk, vmlOk)
	}
	if naaHex != vmlHex {
		t.Errorf("expected same NAA-6 identifier from both formats, got naa=%q vml=%q", naaHex, vmlHex)
	}
}
