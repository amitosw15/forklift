package ontap

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kubev2v/forklift/cmd/vsphere-copy-offload-populator/internal/fcutil"
	"github.com/kubev2v/forklift/cmd/vsphere-copy-offload-populator/internal/logger"
	"github.com/kubev2v/forklift/cmd/vsphere-copy-offload-populator/internal/populator"
	"github.com/kubev2v/forklift/pkg/lib/client/vsphere/vmware"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"k8s.io/klog/v2"
)

const OntapProviderID = "600a0980"

// Ensure NetappClonner implements required interfaces
var _ populator.RDMCapable = &NetappClonner{}
var _ populator.VMDKCapable = &NetappClonner{}
var _ populator.StorageArrayInfoProvider = &NetappClonner{}

type NetappClonner struct {
	api                  api.OntapAPI
	initiatorHostOrGroup string
	arrayInfo            populator.StorageArrayInfo
	log                  klog.Logger
}

// GetStorageArrayInfo returns metadata about the ONTAP array for metric labels.
func (c *NetappClonner) GetStorageArrayInfo() populator.StorageArrayInfo {
	return c.arrayInfo
}

// Map the targetLUN to the initiator group.
func (c *NetappClonner) Map(initatorGroup string, targetLUN populator.LUN, _ populator.MappingContext) (populator.LUN, error) {
	c.log.Info("mapping volume to group", "volume", targetLUN.Name, "group", initatorGroup)

	_, err := c.api.EnsureLunMapped(context.TODO(), initatorGroup, targetLUN.Name)
	if err != nil {
		return populator.LUN{}, fmt.Errorf("Failed to map lun path %s to group %s: %w ", targetLUN.Name, initatorGroup, err)
	}

	c.log.Info("volume mapped successfully", "volume", targetLUN.Name, "group", initatorGroup)
	return targetLUN, nil
}

func (c *NetappClonner) UnMap(initatorGroup string, targetLUN populator.LUN, _ populator.MappingContext) error {
	c.log.Info("unmapping volume from group", "volume", targetLUN.Name, "group", initatorGroup)

	err := c.api.LunUnmap(context.TODO(), initatorGroup, targetLUN.Name)
	if err != nil {
		return err
	}

	c.log.Info("volume unmapped successfully", "volume", targetLUN.Name, "group", initatorGroup)
	return nil
}

func (c *NetappClonner) MapTarget(targetLUN populator.LUN, context populator.MappingContext) (populator.LUN, error) {
	return c.Map(c.initiatorHostOrGroup, targetLUN, context)
}

func (c *NetappClonner) UnmapTarget(targetLUN populator.LUN, context populator.MappingContext) error {
	return c.UnMap(c.initiatorHostOrGroup, targetLUN, context)
}

func (c *NetappClonner) EnsureClonnerIgroup(initiatorGroup string, adapterIds []string) (populator.MappingContext, error) {
	c.log.Info("ensuring initiator group", "group", initiatorGroup, "adapters", adapterIds)

	// Detect protocol from adapters to avoid mixed protocol groups
	protocol := "mixed"
	for _, id := range adapterIds {
		if strings.HasPrefix(id, "fc.") || strings.HasPrefix(id, "20") {
			protocol = "fcp" // NetApp uses 'fcp' for Fibre Channel protocol
			break
		}
		if strings.HasPrefix(id, "iqn.") || strings.HasPrefix(id, "eui.") || strings.HasPrefix(id, "nqn.") {
			protocol = "iscsi"
			break
		}
	}

	// Append protocol suffix to avoid mixed protocol igroup errors
	c.initiatorHostOrGroup = initiatorGroup + "-" + protocol
	c.log.V(2).Info("detected protocol", "protocol", protocol, "final_group", c.initiatorHostOrGroup)

	// esxs needs "vmware" as the group protocol.
	err := c.api.IgroupCreate(context.Background(), c.initiatorHostOrGroup, protocol, "vmware")
	if err != nil {
		// TODO ignore if exists error? with ontap there is no error
		return nil, fmt.Errorf("failed adding igroup %w", err)
	}

	atLeastOneAdded := false

	for _, adapterId := range adapterIds {
		// Convert FC initiators from ESXi format (fc.WWNN:WWPN) to ONTAP format (colon-separated WWPN)
		ontapInitiator := adapterId
		if strings.HasPrefix(adapterId, "fc.") {
			converted, convErr := fcutil.ExtractAndFormatWWPN(adapterId)
			if convErr != nil {
				c.log.Info("failed to convert FC adapter to ONTAP format", "adapter", adapterId, "err", convErr)
				continue
			}
			c.log.V(2).Info("converted FC adapter to ONTAP format", "adapter", adapterId, "converted", converted)
			ontapInitiator = converted
		}

		err = c.api.EnsureIgroupAdded(context.Background(), c.initiatorHostOrGroup, ontapInitiator)
		if err != nil {
			c.log.Info("failed adding initiator to igroup", "initiator", ontapInitiator, "err", err)
			if strings.Contains(err.Error(), "[409]") {
				// duplicate initiator in a group
				atLeastOneAdded = true
			}
			continue
		}
		atLeastOneAdded = true
	}
	if !atLeastOneAdded {
		return nil, fmt.Errorf("failed adding any host to igroup")
	}

	c.log.Info("initiator group ready", "group", c.initiatorHostOrGroup)
	return nil, nil
}

func NewNetappClonner(hostname, username, password string) (NetappClonner, error) {
	log := logger.New("ontap")

	// additional ontap values should be passed as env variables using prefix ONTAP_
	svm := os.Getenv("ONTAP_SVM")
	config := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{},
		ManagementLIF:             hostname,
		Username:                  username,
		Password:                  password,
		LimitAggregateUsage:       "",
		SVM:                       svm,
	}

	client, err := api.NewRestClientFromOntapConfig(context.TODO(), &config)
	if err != nil {
		log.V(2).Info("ONTAP client initialization error details", "err", err)
		return NetappClonner{}, fmt.Errorf("failed to initialize ONTAP client (common causes: incorrect password, invalid SVM name, network connectivity): %w", err)
	}

	nc := NetappClonner{
		api: client,
		arrayInfo: populator.StorageArrayInfo{
			Vendor:  "NetApp",
			Product: "ONTAP",
		},
		log: log,
	}

	// Fetch ONTAP API version
	ontapVersion, err := client.APIVersion(context.TODO())
	if err != nil {
		log.Info("failed to get ONTAP version for metrics", "err", err)
	} else {
		nc.arrayInfo.Version = ontapVersion
		log.V(2).Info("ONTAP array info", "vendor", nc.arrayInfo.Vendor, "product", nc.arrayInfo.Product, "version", nc.arrayInfo.Version)
	}

	return nc, nil
}

// parseInternalIDToLunPath converts internalID format to LUN path format.
// internalID format: /svm/{svm}/flexvol/{flexvol}/lun/{lun}
// LUN path format: /vol/{flexvol}/{lun}
func parseInternalIDToLunPath(internalID string) (string, error) {
	// Find the flexvol section
	_, reminder, ok := strings.Cut(internalID, "/flexvol/")
	if !ok {
		return "", fmt.Errorf("invalid internalID format: missing /flexvol/ in %s", internalID)
	}

	// Validate that the remainder contains /lun/
	if !strings.Contains(reminder, "/lun/") {
		return "", fmt.Errorf("invalid internalID format: missing /lun/ in %s", internalID)
	}

	flexVol, lunName, ok := strings.Cut(reminder, "/lun/")
	if !ok {
		return "", fmt.Errorf("invalid internalID format: missing /lun/ in %s", internalID)
	}

	// Prepend "/vol/" to convert the format
	return fmt.Sprintf("/vol/%s/%s", flexVol, lunName), nil
}

func (c *NetappClonner) ResolvePVToLUN(pv populator.PersistentVolume) (populator.LUN, error) {
	c.log.Info("resolving PV to LUN", "pv", pv.Name, "volume_handle", pv.VolumeHandle)

	var lunPath string

	// Check for ontap-san-economy storage class (has internalID with full path)
	if internalID, ok := pv.VolumeAttributes["internalID"]; ok {
		c.log.V(2).Info("using economy storage class path resolution", "pv", pv.Name, "internal_id", internalID)
		parsedPath, err := parseInternalIDToLunPath(internalID)
		if err != nil {
			return populator.LUN{}, fmt.Errorf("failed to parse internalID for PV %s: %w", pv.Name, err)
		}
		lunPath = parsedPath
		c.log.V(2).Info("parsed LUN path from internalID", "lun_path", lunPath)
	} else {
		// Standard ontap-san storage class - uses dedicated FlexVol with lun0
		internalName, ok := pv.VolumeAttributes["internalName"]
		if !ok {
			return populator.LUN{}, fmt.Errorf("neither internalID nor internalName attribute found on PersistentVolume %s", pv.Name)
		}
		lunPath = fmt.Sprintf("/vol/%s/lun0", internalName)
		c.log.V(2).Info("using standard storage class LUN path", "lun_path", lunPath)
	}

	l, err := c.api.LunGetByName(context.Background(), lunPath)
	if err != nil {
		return populator.LUN{}, fmt.Errorf("failed to get LUN at path %s: %w", lunPath, err)
	}

	// in RHEL lsblk needs that swap. In fedora it doesn't
	//serialNumber :=  strings.ReplaceAll(l.SerialNumber, "?", "\\\\x3f")
	naa := fmt.Sprintf("naa.%s%x", OntapProviderID, l.SerialNumber)
	lun := populator.LUN{Name: l.Name, VolumeHandle: pv.VolumeHandle, SerialNumber: l.SerialNumber, NAA: naa}

	c.log.Info("LUN resolved", "lun", lun.Name, "naa", lun.NAA, "serial", lun.SerialNumber)
	return lun, nil
}

func (c *NetappClonner) Get(lun populator.LUN, _ populator.MappingContext) (string, error) {
	// this code is from netapp/trident/storage_drivers/ontap/ontap_common.go
	// FIXME - this ips list needs to be intersected with the list of reporting
	// nodes for the LUN? see c.api.LunMapGetReportingNodes
	ips, err := c.api.NetInterfaceGetDataLIFs(context.Background(), "iscsi")
	if err != nil || len(ips) < 1 {
		return "", err
	}
	return ips[0], nil
}

func (c *NetappClonner) CurrentMappedGroups(targetLUN populator.LUN, _ populator.MappingContext) ([]string, error) {
	c.log.V(2).Info("querying current mapped groups", "lun", targetLUN.Name)

	lunMappedIgroups, err := c.api.LunListIgroupsMapped(context.Background(), targetLUN.Name)
	if err != nil {
		return nil, fmt.Errorf("Failed to get mapped luns by path %s: %w ", targetLUN.Name, err)
	}

	c.log.V(2).Info("found mapped groups", "lun", targetLUN.Name, "groups", lunMappedIgroups)
	return lunMappedIgroups, nil
}

// RDMCopy performs a copy operation for RDM-backed disks using NetApp ONTAP APIs.
// It resolves the RDM device to a source LUN, sets the target LUN's fstype attribute
// to "raw" to prevent filesystem detection during import, then clones the LUN.
func (c *NetappClonner) RDMCopy(vsphereClient vmware.Client, vmId string, sourceVMDKFile string, persistentVolume populator.PersistentVolume, progress chan<- uint64) error {
	c.log.Info("RDM copy started", "vm", vmId, "source", sourceVMDKFile)

	backing, err := vsphereClient.GetVMDiskBacking(context.Background(), vmId, sourceVMDKFile)
	if err != nil {
		return fmt.Errorf("failed to get RDM disk backing info: %w", err)
	}

	if !backing.IsRDM {
		return fmt.Errorf("disk %s is not an RDM disk", sourceVMDKFile)
	}

	c.log.Info("found RDM device", "device", backing.DeviceName)

	sourceLUN, err := c.resolveRDMToLUN(backing.DeviceName)
	if err != nil {
		return fmt.Errorf("failed to resolve RDM device to source LUN: %w", err)
	}

	c.log.Info("resolving target PV to LUN", "pv", persistentVolume.Name)
	targetLUN, err := c.ResolvePVToLUN(persistentVolume)
	if err != nil {
		return fmt.Errorf("failed to resolve target volume: %w", err)
	}

	progress <- 10

	// NiMo's trick: set fstype to "raw" so Trident doesn't try to detect/create
	// a filesystem on the target LUN during import.
	if err := c.setLunFsTypeToRaw(targetLUN.Name); err != nil {
		return fmt.Errorf("failed to set LUN fstype to raw: %w", err)
	}

	c.log.Info("cloning LUN", "source", sourceLUN.Name, "target", targetLUN.Name)
	if err := c.performBlockVolumeImport(sourceLUN.Name, targetLUN.Name); err != nil {
		return fmt.Errorf("LUN clone failed: %w", err)
	}

	progress <- 100

	c.log.Info("RDM copy completed successfully")
	return nil
}

func (c *NetappClonner) resolveRDMToLUN(deviceName string) (populator.LUN, error) {
	c.log.V(2).Info("resolving RDM device to LUN", "device", deviceName)

	serial, err := extractSerialFromNAA(deviceName)
	if err != nil {
		c.log.Info("could not extract serial from NAA, trying brute-force search", "device", deviceName, "err", err)
		return c.findLUNByDeviceName(deviceName)
	}

	c.log.V(2).Info("finding LUN by serial", "serial", serial)
	return c.findLUNBySerial(serial)
}

// extractSerialFromNAA extracts the serial from a NAA/VML device identifier.
// ONTAP NAA format: naa.600a0980<serial_hex> or vml.0200...<hex_encoded>
func extractSerialFromNAA(naa string) (string, error) {
	naa = strings.ToLower(naa)

	// Strip common prefixes
	naa = strings.TrimPrefix(naa, "vml.")
	naa = strings.TrimPrefix(naa, "naa.")

	// Remove any leading zeros and length bytes from VML encoding
	providerIDLower := strings.ToLower(OntapProviderID)
	idx := strings.Index(naa, providerIDLower)
	if idx < 0 {
		return "", fmt.Errorf("NAA %s does not contain ONTAP provider ID %s", naa, OntapProviderID)
	}

	serial := naa[idx+len(providerIDLower):]
	if serial == "" {
		return "", fmt.Errorf("could not extract serial from NAA %s", naa)
	}

	return serial, nil
}

func (c *NetappClonner) findLUNBySerial(serial string) (populator.LUN, error) {
	luns, err := c.api.LunList(context.Background(), "*")
	if err != nil {
		return populator.LUN{}, fmt.Errorf("failed to list LUNs: %w", err)
	}

	serial = strings.ToLower(serial)
	for _, lun := range luns {
		if strings.ToLower(lun.SerialNumber) == serial {
			naa := fmt.Sprintf("naa.%s%s", OntapProviderID, strings.ToLower(lun.SerialNumber))
			return populator.LUN{
				Name:         lun.Name,
				SerialNumber: lun.SerialNumber,
				NAA:          naa,
			}, nil
		}
	}

	return populator.LUN{}, fmt.Errorf("no LUN found with serial %s", serial)
}

func (c *NetappClonner) findLUNByDeviceName(deviceName string) (populator.LUN, error) {
	luns, err := c.api.LunList(context.Background(), "*")
	if err != nil {
		return populator.LUN{}, fmt.Errorf("failed to list LUNs: %w", err)
	}

	deviceName = strings.ToLower(deviceName)
	for _, lun := range luns {
		serialLower := strings.ToLower(lun.SerialNumber)
		if serialLower != "" && strings.Contains(deviceName, serialLower) {
			naa := fmt.Sprintf("naa.%s%s", OntapProviderID, serialLower)
			c.log.Info("found matching LUN by device name", "lun", lun.Name, "device", deviceName)
			return populator.LUN{
				Name:         lun.Name,
				SerialNumber: lun.SerialNumber,
				NAA:          naa,
			}, nil
		}
	}

	return populator.LUN{}, fmt.Errorf("could not find LUN matching RDM device %s", deviceName)
}

// setLunFsTypeToRaw sets com.netapp.ndvp.fstype to "raw" on the target LUN so
// Trident treats it as a raw block device and skips filesystem creation.
func (c *NetappClonner) setLunFsTypeToRaw(lunPath string) error {
	c.log.Info("setting LUN fstype to raw", "lun", lunPath)
	return c.api.LunSetAttribute(context.Background(), lunPath, "com.netapp.ndvp.fstype", "raw", "", "")
}

func (c *NetappClonner) performBlockVolumeImport(sourceLUNPath, targetLUNPath string) error {
	c.log.Info("performing LUN clone", "source", sourceLUNPath, "target", targetLUNPath)

	sourceLUN, err := c.api.LunGetByName(context.Background(), sourceLUNPath)
	if err != nil {
		return fmt.Errorf("failed to get source LUN info: %w", err)
	}

	// LunCloneCreate args: flexvol (target), source, lunName, qosPolicyGroup
	// Extract the flexvol from the target path: /vol/<flexvol>/<lun>
	parts := strings.Split(strings.TrimPrefix(targetLUNPath, "/vol/"), "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid target LUN path format: %s", targetLUNPath)
	}
	flexvol := parts[0]

	return c.api.LunCloneCreate(context.Background(), flexvol, sourceLUN.Name, targetLUNPath, api.QosPolicyGroup{})
}
