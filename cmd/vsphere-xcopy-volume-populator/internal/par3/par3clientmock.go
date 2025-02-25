package par3

import (
	"context"
	"fmt"
	"log"

	"github.com/kubev2v/forklift/cmd/vsphere-xcopy-volume-populator/internal/populator"
)

type MockPar3Client struct {
	SessionKey string
	Volumes    map[string]populator.LUN
	VLUNs      map[string][]VLun
	Hosts      map[string]string
	HostSets   map[string][]string
}

func NewMockPar3Client() *MockPar3Client {
	return &MockPar3Client{
		SessionKey: "mock-session-key",
		Volumes:    make(map[string]populator.LUN),
		VLUNs:      make(map[string][]VLun),
		Hosts:      make(map[string]string),
		HostSets:   make(map[string][]string),
	}
}

func (m *MockPar3Client) GetSessionKey() (string, error) {
	log.Println("Mock: GetSessionKey called")
	return m.SessionKey, nil
}

func (m *MockPar3Client) EnsureHostWithIqn(iqn string) (string, error) {
	for hostName, existingIQN := range m.Hosts {
		if existingIQN == iqn {
			return hostName, nil
		}
	}

	hostName := fmt.Sprintf("mock-host-%s", iqn)
	m.Hosts[hostName] = iqn
	log.Printf("Mock: Created host %s with IQN %s", hostName, iqn)
	return hostName, nil
}

func (m *MockPar3Client) EnsureHostSetExists(hostSetName string) error {
	if _, exists := m.HostSets[hostSetName]; !exists {
		m.HostSets[hostSetName] = []string{}
		log.Printf("Mock: Created host set %s", hostSetName)
	}
	return nil
}

func (m *MockPar3Client) AddHostToHostSet(hostSetName string, hostName string) error {
	if _, exists := m.HostSets[hostSetName]; !exists {
		return fmt.Errorf("mock: host set %s does not exist", hostSetName)
	}

	for _, existingHost := range m.HostSets[hostSetName] {
		if existingHost == hostName {
			return nil
		}
	}

	m.HostSets[hostSetName] = append(m.HostSets[hostSetName], hostName)
	log.Printf("Mock: Added host %s to host set %s", hostName, hostSetName)
	return nil
}

func (m *MockPar3Client) EnsureLunMapped(initiatorGroup string, targetLUN *populator.LUN) error {
	if _, exists := m.Volumes[targetLUN.Name]; !exists {
		return fmt.Errorf("mock: volume %s does not exist", targetLUN.Name)
	}

	vlun := VLun{
		VolumeName: targetLUN.Name,
		LUN:        len(m.VLUNs[initiatorGroup]) + 1,
		Hostname:   initiatorGroup,
	}

	m.VLUNs[initiatorGroup] = append(m.VLUNs[initiatorGroup], vlun)
	log.Printf("Mock: EnsureLunMapped -> Volume %s mapped to initiator group %s with LUN ID %d", targetLUN.Name, initiatorGroup, vlun.LUN)
	return nil
}

func (m *MockPar3Client) LunUnmap(ctx context.Context, initiatorGroupName, lunName string) error {
	vluns, exists := m.VLUNs[initiatorGroupName]
	if !exists {
		return fmt.Errorf("mock: no VLUNs found for initiator group %s", initiatorGroupName)
	}

	for i, vlun := range vluns {
		if vlun.VolumeName == lunName {
			m.VLUNs[initiatorGroupName] = append(vluns[:i], vluns[i+1:]...)
			log.Printf("Mock: LunUnmap -> Volume %s unmapped from initiator group %s", lunName, initiatorGroupName)
			return nil
		}
	}

	return fmt.Errorf("mock: LUN %s not found for initiator group %s", lunName, initiatorGroupName)
}

func (m *MockPar3Client) GetLunDetailsByVolumeName(lunName string, lun *populator.LUN) error {
	if volume, exists := m.Volumes[lunName]; exists {
		*lun = volume

		log.Printf("Mock: GetLunDetailsByVolumeName -> Found volume %s", lunName)
		return nil
	}

	return fmt.Errorf("mock: volume %s not found", lunName)
}

func (m *MockPar3Client) CurrentMappedGroups(volumeName string) ([]string, error) {
	var groups []string

	for group, vluns := range m.VLUNs {
		for _, vlun := range vluns {
			if vlun.VolumeName == volumeName {
				groups = append(groups, group)
			}
		}
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("mock: no mapped groups found for volume %s", volumeName)
	}

	log.Printf("Mock: CurrentMappedGroups -> Volume %s is mapped to groups: %v", volumeName, groups)
	return groups, nil
}
