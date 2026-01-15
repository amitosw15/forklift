package infinibox

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	"github.com/kubev2v/forklift/cmd/vsphere-xcopy-volume-populator/internal/fcutil"
	"github.com/kubev2v/forklift/cmd/vsphere-xcopy-volume-populator/internal/populator"
	"k8s.io/klog/v2"
)

const (
	hostIDContextKey      string = "hostID"
	esxLogicalHostNameKey string = "esxLogicalHostName"
	esxRealHostNameKey    string = "esxRealHostName"
	ocpRealHostNameKey    string = "ocpRealHostName"
)

type InfiniboxClonner struct {
	api iboxapi.Client
}

func (c *InfiniboxClonner) Map(initiatorGroup string, targetLUN populator.LUN, mappingContext populator.MappingContext) (populator.LUN, error) {
	if mappingContext == nil {
		return targetLUN, fmt.Errorf("mapping context is required")
	}

	hostName := ""
	if initiatorGroup != mappingContext[esxLogicalHostNameKey] {
		hostName = mappingContext[ocpRealHostNameKey].(string)
	} else {
		hostName = mappingContext[esxRealHostNameKey].(string)
	}
	klog.Infof("mapping volume %s to initiator-group %s", targetLUN.Name, hostName)

	host, err := c.api.GetHostByName(hostName)
	if err != nil {
		return targetLUN, fmt.Errorf("failed to find host for host name %s: %w", hostName, err)
	}

	volumeID, err := strconv.Atoi(targetLUN.LDeviceID)
	// Idempotency: check if already mapped
	existingMappings, err := c.api.GetLunsByVolume(volumeID)
	if err == nil {
		for _, mapping := range existingMappings {
			if mapping.HostID == host.ID {
				klog.Infof("Volume %s already mapped to initiator group %s", targetLUN.Name, hostName)
				return targetLUN, nil
			}
		}
	}

	_, err = c.api.MapVolumeToHost(host.ID, volumeID, 0)
	if err != nil {
		return targetLUN, fmt.Errorf("failed to map volume %s to host %s: %w", targetLUN.Name, hostName, err)
	}

	klog.Infof("Successfully mapped volume %s to initiator group %s", targetLUN.Name, hostName)
	return targetLUN, nil
}

func (c *InfiniboxClonner) UnMap(initiatorGroup string, targetLUN populator.LUN, mappingContext populator.MappingContext) error {
	if mappingContext == nil {
		return fmt.Errorf("mapping context is required")
	}

	hostName := ""
	if initiatorGroup != mappingContext[esxLogicalHostNameKey] {
		hostName = mappingContext[ocpRealHostNameKey].(string)
	} else {
		hostName = mappingContext[esxRealHostNameKey].(string)
	}
	klog.Infof("unmapping volume %s from initiator-group %s", targetLUN.Name, hostName)

	host, err := c.api.GetHostByName(hostName)
	if err != nil {
		return fmt.Errorf("failed to find host for host name %s: %w", hostName, err)
	}

	volumeID, err := strconv.Atoi(targetLUN.LDeviceID)
	if err != nil {
		return fmt.Errorf("failed to convert volume ID %s to integer: %w", targetLUN.LDeviceID, err)
	}

	_, err = c.api.UnMapVolumeFromHost(host.ID, volumeID)
	if err != nil {
		return fmt.Errorf("failed to unmap volume %s from host %s: %w", targetLUN.Name, hostName, err)
	}

	klog.Infof("Successfully unmapped volume %s from initiator group %s", targetLUN.Name, hostName)
	return nil
}

func (c *InfiniboxClonner) EnsureClonnerIgroup(initiatorGroup string, adapterIds []string) (populator.MappingContext, error) {
	hosts, err := c.api.GetAllHosts()
	if err != nil {
		return nil, fmt.Errorf("failed to get all hosts: %w", err)
	}

	for _, host := range hosts {
		for _, port := range host.Ports {
			for _, adapterId := range adapterIds {
				matched := false
				// For FC adapters, extract WWPN and compare using normalized comparison
				if strings.HasPrefix(adapterId, "fc.") {
					wwpn, err := fcutil.ExtractWWPN(adapterId)
					klog.Infof("Extracted WWPN from adapter ID %s: %s", adapterId, wwpn)
					if err != nil {
						klog.Warningf("Failed to extract WWPN from adapter ID %s: %v", adapterId, err)
						continue
					}
					// Compare normalized WWNs to handle different formatting
					klog.Infof("Comparing normalized WWNs: %s with %s", wwpn, port.Address)
					if fcutil.CompareWWNs(wwpn, port.Address) {
						matched = true
					}
				} else {
					// For iSCSI or other protocols, do direct comparison
					if port.Address == adapterId {
						matched = true
					}
				}

				if matched {
					klog.Infof("Found host %s with adapter ID %s (port address: %s)", host.Name, adapterId, port.Address)
					return createMappingContext(&host, initiatorGroup), nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no host found with adapter IDs %v", adapterIds)
}

func createMappingContext(host *iboxapi.Host, initiatorGroup string) populator.MappingContext {
	return populator.MappingContext{
		hostIDContextKey:      host.ID,
		esxLogicalHostNameKey: initiatorGroup,
		esxRealHostNameKey:    host.Name,
	}
}

func NewInfiniboxClonner(hostname, username, password string, insecure bool) (InfiniboxClonner, error) {
	// Create credentials
	creds := iboxapi.Credentials{
		Username: username,
		Password: password,
		URL:      hostname,
	}

	// Create logger (using klog adapter)
	logger := logr.Discard()

	// Create InfiniBox client
	client := iboxapi.NewIboxClient(logger, creds)

	return InfiniboxClonner{
		api: client,
	}, nil
}

func (c *InfiniboxClonner) ResolvePVToLUN(pv populator.PersistentVolume) (populator.LUN, error) {
	volumeAttributes := pv.VolumeAttributes
	volumeName := volumeAttributes["Name"]
	volume, err := c.api.GetVolumeByName(volumeName)
	if err != nil {
		return populator.LUN{}, fmt.Errorf("failed to get volume by name %s: %w", volumeName, err)
	}
	serial := volume.Serial
	protocol := volumeAttributes["storage_protocol"]
	protocolPrefix := ""
	switch protocol {
	case "iscsi":
		protocolPrefix = "iqn"
	default:
		protocolPrefix = "naa"
	}
	IQN := fmt.Sprintf("%s.%s", protocolPrefix, serial)
	NAA := fmt.Sprintf("naa.6%s", serial)
	klog.Infof("Successfully resolved volume %s", volumeName)

	lun := populator.LUN{
		Name:         volumeName,
		LDeviceID:    strconv.Itoa(volume.ID),
		VolumeHandle: pv.VolumeHandle,
		SerialNumber: serial,
		IQN:          IQN,
		NAA:          NAA,
	}
	return lun, nil
}

func (c *InfiniboxClonner) CurrentMappedGroups(targetLUN populator.LUN, mappingContext populator.MappingContext) ([]string, error) {
	volumeID := targetLUN.LDeviceID

	volumeIDInt, err := strconv.Atoi(volumeID)
	if err != nil {
		return nil, fmt.Errorf("invalid volume ID '%s', expected integer volume ID: %w", volumeID, err)
	}

	klog.Infof("Checking mappings for volume ID %d (LDeviceID: %s)", volumeIDInt, volumeID)
	lunInfos, err := c.api.GetLunsByVolume(volumeIDInt)
	if err != nil {
		return nil, fmt.Errorf("failed to get LUN mappings for volume %s: %w", volumeID, err)
	}

	klog.Infof("Found %d LUN mappings for volume %s", len(lunInfos), volumeID)

	if len(lunInfos) == 0 {
		klog.Infof("Volume %s is not mapped to any hosts", volumeID)
		return []string{}, nil
	}

	// Log all LUN mapping details for debugging
	for i, lunInfo := range lunInfos {
		klog.Infof("LUN mapping %d: HostID=%d, HostClusterID=%d, Clustered=%v, Lun=%d",
			i+1, lunInfo.HostID, lunInfo.HostClusterID, lunInfo.CLustered, lunInfo.Lun)
	}																																										
	allHosts, err := c.api.GetAllHosts()
	if err != nil {
		return nil, fmt.Errorf("failed to get all hosts: %w", err)
	}

	klog.Infof("Retrieved %d hosts from InfiniBox", len(allHosts))
	hostByID := make(map[int]*iboxapi.Host)
	time.Sleep(10 * time.Second)
	for i := range allHosts {
		hostByID[allHosts[i].ID] = &allHosts[i]
		klog.Infof("Host ID %d: %s", allHosts[i].ID, allHosts[i].Name)
	}

	mappedHosts := make([]string, 0, len(lunInfos))
	hostIDsProcessed := make(map[int]bool)

	for _, lunInfo := range lunInfos {
		if hostIDsProcessed[lunInfo.HostID] {
			continue
		}

		if lunInfo.CLustered {
			klog.Warningf("Volume %s is mapped to host cluster %d (cluster mappings not fully supported)",
				volumeID, lunInfo.HostClusterID)
			continue
		}

		host, exists := hostByID[lunInfo.HostID]
		if !exists {
			klog.Warningf("Failed to find host info for host ID %d. Available host IDs: %v", lunInfo.HostID, getHostIDs(allHosts))
			// Check if this might be a host cluster ID instead
			if lunInfo.HostClusterID > 0 {
				klog.Warningf("LUN mapping has HostClusterID=%d, but HostID=%d was not found. This might be a cluster mapping.", lunInfo.HostClusterID, lunInfo.HostID)
			}
			continue
		}

		mappedHosts = append(mappedHosts, host.Name)
		hostIDsProcessed[lunInfo.HostID] = true

		if _, ok := mappingContext[ocpRealHostNameKey]; !ok {
			mappingContext[ocpRealHostNameKey] = host.Name
			klog.Infof("Volume %s is currently mapped to host: %s", volumeID, host.Name)
			return mappedHosts, nil
		}

		klog.Infof("Volume %s is mapped to host %s (ID: %d) as LUN %d",
			volumeID, host.Name, lunInfo.HostID, lunInfo.Lun)
	}

	if len(mappedHosts) == 0 {
		return nil, fmt.Errorf("volume %s is not mapped to any host", volumeID)
	}

	return mappedHosts, nil
}

// getHostIDs returns a slice of all host IDs for debugging purposes
func getHostIDs(hosts []iboxapi.Host) []int {
	ids := make([]int, 0, len(hosts))
	for _, host := range hosts {
		ids = append(ids, host.ID)
	}
	return ids
}
