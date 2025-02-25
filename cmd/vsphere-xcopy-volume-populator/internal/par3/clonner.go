package par3

import (
	"context"
	"fmt"
	"github.com/kubev2v/forklift/cmd/vsphere-xcopy-volume-populator/internal/populator"
)

const PROVIDER_ID = "60002ac"

type Par3Clonner struct {
	client Par3Client
}

func NewPar3Clonner(storageHostname, storageUsername, storagePassword string) (Par3Clonner, error) {
	clon := NewPar3ClientWsImpl(storageHostname, storageUsername, storagePassword)
	return Par3Clonner{
		client: &clon,
	}, nil
}

// EnsureClonnerIgroup creates or update an initiator group with the clonnerIqn
func (c *Par3Clonner) EnsureClonnerIgroup(initiatorGroup string, clonnerIqn string) error {
	hostName, err := c.client.EnsureHostWithIqn(clonnerIqn)
	if err != nil {
		return fmt.Errorf("failed to ensure host with IQN: %w", err)
	}

	err = c.client.EnsureHostSetExists(initiatorGroup)
	if err != nil {
		return fmt.Errorf("failed to ensure host set: %w", err)
	}

	err = c.client.AddHostToHostSet(initiatorGroup, hostName)
	if err != nil {
		return fmt.Errorf("failed to add host to host set: %w", err)
	}

	return nil
}

// Map is responsible to mapping an initiator group to a LUN
func (c *Par3Clonner) Map(initiatorGroup string, targetLUN *populator.LUN) error {
	return c.client.EnsureLunMapped(initiatorGroup, targetLUN)
}

// UnMap is responsible to unmapping an initiator group from a LUN
func (c *Par3Clonner) UnMap(initiatorGroup string, targetLUN populator.LUN) error {
	return c.client.LunUnmap(context.TODO(), initiatorGroup, targetLUN.Name)
}

// Return initiatorGroups the LUN is mapped to
func (p *Par3Clonner) CurrentMappedGroups(targetLUN populator.LUN) ([]string, error) {
	res, err := p.client.CurrentMappedGroups(targetLUN.Name)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get current mapped groups: %w", err)
	}
	return res, nil
}

func (c *Par3Clonner) ResolveVolumeHandleToLUN(volumeHandle string) (populator.LUN, error) {
	lun := populator.LUN{VolumeHandle: volumeHandle}
	err := c.client.GetLunDetailsByVolumeName(volumeHandle, &lun)
	if err != nil {
		return populator.LUN{}, err
	}
	return lun, nil
}
