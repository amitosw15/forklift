package vsphere

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	model "github.com/kubev2v/forklift/pkg/controller/provider/web/vsphere"
	"github.com/kubev2v/forklift/pkg/settings"
	"github.com/kubev2v/forklift/pkg/storage/utils"
	core "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// naaVendorEntry maps an NAA hex prefix to a StorageVendorProduct.
type naaVendorEntry struct {
	prefix string
	vendor api.StorageVendorProduct
}

// naaVendorPrefixes maps NAA identifier prefixes to StorageVendorProduct values.
// Used to identify which storage array backs an RDM disk based on its NAA
// (Network Address Authority) identifier. Prefixes are IEEE OUI-based and
// ordered longest-first so the most specific match wins.
// Multiple entries may map to the same vendor when a vendor has multiple OUI
// registrations (e.g. Dell EMC has separate OUIs for Symmetrix and VMAX families).
var naaVendorPrefixes = []naaVendorEntry{
	// Pure Storage (OUI 24a937)
	{prefix: "624a9370", vendor: api.StorageVendorProductPureFlashArray},
	// Dell EMC PowerStore (OUI 8ccf09)
	{prefix: "68ccf098", vendor: api.StorageVendorProductPowerStore},
	// Dell EMC PowerMax / Symmetrix (OUI 000097)
	{prefix: "6000097", vendor: api.StorageVendorProductPowerMax},
	// Dell EMC PowerMax / VMAX (OUI 006048)
	{prefix: "6006048", vendor: api.StorageVendorProductPowerMax},
	// Hitachi Vantara (OUI 0060e8)
	{prefix: "60060e8", vendor: api.StorageVendorProductVantara},
	// NetApp ONTAP (OUI 00a098)
	{prefix: "600a098", vendor: api.StorageVendorProductOntap},
	// IBM FlashSystem (OUI 005076)
	{prefix: "6005076", vendor: api.StorageVendorProductFlashSystem},
	// HPE Primera / 3PAR (OUI 0002ac)
	{prefix: "60002ac", vendor: api.StorageVendorProductPrimera3Par},
	// Infinidat InfiniBox (OUI 742b0f)
	{prefix: "6742b0f", vendor: api.StorageVendorProductInfinibox},
}

// loadNAAPrefixes returns hardcoded prefixes merged with any admin-supplied
// overrides from the NAA OUI ConfigMap. ConfigMap entries add to (or override)
// hardcoded ones. Returns the base list if the ConfigMap doesn't exist or
// the setting is empty.
func loadNAAPrefixes(k8sClient k8sclient.Client) []naaVendorEntry {
	merged := slices.Clone(naaVendorPrefixes)
	cmName := settings.Settings.NAAOUIMapConfigMap
	if cmName == "" {
		return merged
	}
	cm := &core.ConfigMap{}
	err := k8sClient.Get(context.TODO(),
		k8sclient.ObjectKey{Name: cmName, Namespace: os.Getenv("POD_NAMESPACE")}, cm)
	if err != nil {
		return merged
	}
	for prefix, vendorStr := range cm.Data {
		lowerPrefix := strings.ToLower(prefix)
		vendor := api.StorageVendorProduct(vendorStr)
		found := false
		for i, e := range merged {
			if e.prefix == lowerPrefix {
				merged[i].vendor = vendor
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, naaVendorEntry{prefix: lowerPrefix, vendor: vendor})
		}
	}
	return merged
}

// vendorFromNAA extracts the storage vendor from an RDM device's NAA identifier.
// The deviceName may be:
//   - a bare NAA (e.g. "naa.6000097...")
//   - a full device path (e.g. "/vmfs/devices/disks/naa.6000097...")
//   - a VML identifier (e.g. "vml.02006a000068ccf098...") where the NAA is embedded
//
// Returns the matched vendor and true, or empty string and false if no known
// prefix matches.
func vendorFromNAA(deviceName string, prefixes []naaVendorEntry) (api.StorageVendorProduct, bool) {
	naa, ok := utils.NAAHexFromDeviceName(deviceName)
	if !ok {
		return "", false
	}
	for _, entry := range prefixes {
		if strings.HasPrefix(naa, entry.prefix) {
			return entry.vendor, true
		}
	}
	return "", false
}

// findStorageMapEntriesForVendor searches the storage map for all entries whose
// offload plugin matches the given vendor.
func findStorageMapEntriesForVendor(storageMap []api.StoragePair, vendor api.StorageVendorProduct) []*api.StoragePair {
	var matches []*api.StoragePair
	for i := range storageMap {
		entry := &storageMap[i]
		if entry.OffloadPlugin == nil || entry.OffloadPlugin.VSphereXcopyPluginConfig == nil {
			continue
		}
		if entry.OffloadPlugin.VSphereXcopyPluginConfig.StorageVendorProduct == vendor {
			matches = append(matches, entry)
		}
	}
	return matches
}

// datastoreFinder can look up a Datastore by ref. Satisfied by web.Client.
type datastoreFinder interface {
	Find(resource interface{}, ref ref.Ref) error
}

// disambiguateRDMByNAA picks the storage map entry whose source datastore's
// backing devices share the longest NAA common prefix with the RDM device.
// Two LUNs on the same array share the OUI + array serial portion of the NAA,
// giving a much longer common prefix than LUNs on different arrays.
func disambiguateRDMByNAA(
	inventory datastoreFinder,
	candidates []*api.StoragePair,
	rdmDeviceName string,
) (*api.StoragePair, error) {
	rdmNAA, ok := utils.NAAHexFromDeviceName(rdmDeviceName)
	if !ok {
		return nil, fmt.Errorf("cannot extract NAA hex from RDM device %q", rdmDeviceName)
	}

	var bestEntry *api.StoragePair
	bestPrefixLen := 0

	for _, candidate := range candidates {
		ds := &model.Datastore{}
		if err := inventory.Find(ds, candidate.Source); err != nil {
			continue
		}
		for _, backingNAA := range ds.BackingDevicesNames {
			backingHex, ok := utils.NAAHexFromDeviceName(backingNAA)
			if !ok {
				klog.V(2).Infof("could not extract NAA hex from backing device %q, skipping", backingNAA)
				continue
			}
			prefixLen := commonPrefixLen(rdmNAA, backingHex)
			if prefixLen > bestPrefixLen {
				bestPrefixLen = prefixLen
				bestEntry = candidate
			}
		}
	}

	// The OUI alone gives 7 chars of match (type nibble + 6 OUI nibbles).
	// A same-array match should exceed this (array serial adds 8-12+ chars).
	const minArrayMatch = 10
	if bestEntry == nil || bestPrefixLen < minArrayMatch {
		return nil, fmt.Errorf(
			"found %d storage map entries for this vendor but none share an array with RDM device %q "+
				"(best NAA prefix match: %d hex chars)", len(candidates), rdmDeviceName, bestPrefixLen)
	}
	return bestEntry, nil
}

// commonPrefixLen returns the length of the common prefix of two strings.
func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}
