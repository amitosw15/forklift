package powerflex

import (
	"fmt"
	"strings"

	goscaleio "github.com/dell/goscaleio"
	types "github.com/dell/goscaleio/types/v1"
	"github.com/rgolangh/xcopy-volume-populator/internal/populator"
	"k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const XCOPY_CLONNER_GROUP = "xcopy-service-vms"

type PowerFlexClient interface {
	NewVolume() (VolumeWrapper, error)
	GetVolume(param1, volumeID, param3, param4 string, flag bool) ([]*types.Volume, error)
	FindSystem(instanceID, name, href string) (*goscaleio.System, error)
	Authenticate(goscaleioconfigConnect *goscaleio.ConfigConnect) (goscaleio.Cluster, error)
}

type PowerFlexVolume interface {
	MapVolumeSdc(mapVolumeSdcParam *types.MapVolumeSdcParam) error
	UnmapVolumeSdc(unmapVolumeSdcParam *types.UnmapVolumeSdcParam) error
}

type VolumeWrapper struct {
	Volume *types.Volume
	Api    PowerFlexVolume
}
type PowerFlexSystem interface{}

type Clonner struct {
	*goscaleio.Client
	client goscaleio.Client
}

type PowerFlexClonner struct {
	Client PowerFlexClient
	System PowerFlexSystem
}

func (c Clonner) NewVolume() (VolumeWrapper, error) {
	dellVolume := goscaleio.NewVolume(&c.client)
	if dellVolume == nil || dellVolume.Volume == nil {
		return VolumeWrapper{}, fmt.Errorf("Failed to create new volume")
	}
	vol := VolumeWrapper{Volume: dellVolume.Volume, Api: dellVolume}
	return vol, nil
}

func (c *PowerFlexClonner) getVolumeServiceById(targetLUN populator.LUN) (VolumeWrapper, error) {
	volumeId := targetLUN.SerialNumber

	volumes, err := c.Client.GetVolume("", volumeId, "", "", false)
	if err != nil {
		return VolumeWrapper{}, err
	}
	if len(volumes) == 0 {
		return VolumeWrapper{}, fmt.Errorf("no volume found for id %s", volumeId)
	}

	v := volumes[0]
	vol, err := c.Client.NewVolume()
	if err != nil {
		return VolumeWrapper{}, err
	}
	// Create a new volume instance using the underlying client
	vol.Volume = v
	return vol, nil
}

// Map the targetLUN to the sdc. If the the initiator group is "" then
// the default XCOPY_CLONNER_GROUP will be used.
func (c *PowerFlexClonner) Map(initatorGroup string, targetLUN populator.LUN) error {
	volume, err := c.getVolumeServiceById(targetLUN)
	if err != nil {
		return err
	}

	mapVolumeSdcParam := &types.MapVolumeSdcParam{
		SdcID:                 initatorGroup,
		AllowMultipleMappings: "TRUE",
		AllSdcs:               "FALSE",
		AccessMode:            "ReadWrite",
	}
	err = volume.Api.MapVolumeSdc(mapVolumeSdcParam)
	if err != nil {
		return err
	}
	return nil
}

func (c *PowerFlexClonner) UnMap(initatorGroup string, targetLUN populator.LUN) error {
	volume, err := c.getVolumeServiceById(targetLUN)
	if err != nil {
		return err
	}
	unMapVolumeSdcParam := &types.UnmapVolumeSdcParam{
		SdcID:                initatorGroup,
		IgnoreScsiInitiators: "TRUE",
		AllSdcs:              "FALSE",
	}
	err = volume.Api.UnmapVolumeSdc(unMapVolumeSdcParam)
	if err != nil {
		return err
	}
	return nil

}

func (c *PowerFlexClonner) EnsureClonnerIgroup(initiatorGroup string, clonnerIqn string) error {
	return nil
}

func NewPowerFlexClonner(hostname, username, password, systemId string) (PowerFlexClonner, error) {
	dellClient, err := goscaleio.NewClient()
	if err != nil {
		klog.Fatalf("err: %v", err)
	}
	clonner := Clonner{client: *dellClient}
	endpoint := "put_endpoint_here"
	_, err = clonner.client.Authenticate(&goscaleio.ConfigConnect{
		Endpoint: endpoint,
		Username: username,
		Password: password,
	})
	if err != nil {
		klog.Fatalf("error authenticating: %v", err)
	}

	system, err := clonner.client.FindSystem(systemId, "", "")
	if err != nil {
		klog.Fatalf("err: problem getting instance %v", err)
	}

	pc := PowerFlexClonner{Client: clonner, System: system}
	return pc, nil
}

func (c *PowerFlexClonner) ResolveVolumeHandleToLUN(volumeHandle string) (populator.LUN, error) {
	volumeId := strings.Split(volumeHandle, "-")[1]
	lun := populator.LUN{Name: volumeId, SerialNumber: volumeId}
	return lun, nil
}

func ResolvePVToSystemId(pv v1.PersistentVolume) (string, error) {
	// volume.csi.volumeHandle has systemId
	VolumeHandle := pv.Spec.CSI.VolumeHandle
	if VolumeHandle == "" {
		return "", fmt.Errorf("Failed to get the VolumeHandle of the volume from the PV %s", pv.Name)
	}
	systemId := strings.Split(VolumeHandle, "-")[0]
	return systemId, nil
}

func (c *PowerFlexClonner) Get(lun populator.LUN) (string, error) {
	return "", nil
}

func (c *PowerFlexClonner) CurrentMappedGroups(targetLUN populator.LUN) ([]string, error) {
	volume, err := c.getVolumeServiceById(targetLUN)
	if err != nil {
		return nil, err
	}
	if len(volume.Volume.MappedSdcInfo) == 0 {
		return nil, nil
	}
	var mappedSdcIds []string
	for _, sdcInfo := range volume.Volume.MappedSdcInfo {
		mappedSdcIds = append(mappedSdcIds, sdcInfo.SdcID)
	}
	return mappedSdcIds, nil
}
