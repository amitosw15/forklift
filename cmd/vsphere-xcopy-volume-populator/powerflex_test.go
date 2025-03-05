package powerflex_test

import (
	"errors"
	"testing"

	goscaleio "github.com/dell/goscaleio"
	types "github.com/dell/goscaleio/types/v1"
	"github.com/golang/mock/gomock"
	"github.com/rgolangh/xcopy-volume-populator/internal/populator"
	"github.com/rgolangh/xcopy-volume-populator/internal/powerflex"
	"github.com/rgolangh/xcopy-volume-populator/internal/powerflex/mocks"
	"github.com/stretchr/testify/assert"
)

func TestMap(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock objects
	mockClient := mocks.NewMockPowerFlexClient(ctrl)
	mockVolume := mocks.NewMockPowerFlexVolume(ctrl)
	typeVolume := types.Volume{}
	mockSystem := goscaleio.System{}
	initatorGroup := "test-group"
	clonner := powerflex.PowerFlexClonner{Client: mockClient, System: mockSystem}

	targetLUN := populator.LUN{SerialNumber: "12345"} // Ensure this type is correctly imported
	t.Run("map success", func(t *testing.T) {
		// Expect GetVolume to return mockVolume inside a slice
		mockClient.EXPECT().
			GetVolume("", "12345", "", "", false).
			Return([]*types.Volume{&typeVolume}, nil)

		mockClient.EXPECT().
			NewVolume().
			Return(powerflex.VolumeWrapper{Api: mockVolume}, nil)

		// Expect MapVolumeSdc to be called and return nil (success)
		mockVolume.EXPECT().
			MapVolumeSdc(gomock.Any()).
			Return(nil)

		// Create an instance of PowerFlexClonner with the mocked client

		// Execute the function
		err := clonner.Map(initatorGroup, targetLUN)

		// Validate no errors occurred
		assert.NoError(t, err)
	})
	t.Run("GetVolume fails", func(t *testing.T) {
		mockClient.EXPECT().
			GetVolume("", "12345", "", "", false).
			Return(nil, errors.New("failed to get volume"))

		err := clonner.Map(initatorGroup, targetLUN)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get volume")
	})

	// --- Scenario 2: NewVolume fails ---
	t.Run("NewVolume fails", func(t *testing.T) {
		mockClient.EXPECT().
			GetVolume("", "12345", "", "", false).
			Return([]*types.Volume{&typeVolume}, nil)

		mockClient.EXPECT().
			NewVolume().
			Return(powerflex.VolumeWrapper{Api: mockVolume}, nil)

		// Expect MapVolumeSdc to be called and return nil (success)
		mockVolume.EXPECT().
			MapVolumeSdc(gomock.Any()).
			Return(errors.New("failed to create new volume"))

		err := clonner.Map(initatorGroup, targetLUN)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create new volume")
	})
	// --- Scenario 2: NewVolume fails ---
	t.Run("NewVolume fails", func(t *testing.T) {
		mockClient.EXPECT().
			GetVolume("", "12345", "", "", false).
			Return([]*types.Volume{&typeVolume}, nil)

		mockClient.EXPECT().
			NewVolume().
			Return(powerflex.VolumeWrapper{Api: nil}, errors.New("failed to create volume service"))

		// Expect MapVolumeSdc to be called and return nil (success)

		err := clonner.UnMap(initatorGroup, targetLUN)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create volume service")
	})

	// --- Scenario 3: MapVolumeSdc fails ---
	t.Run("MapVolumeSdc fails", func(t *testing.T) {
		mockClient.EXPECT().
			GetVolume("", "12345", "", "", false).
			Return([]*types.Volume{&typeVolume}, nil)

		mockClient.EXPECT().
			NewVolume().
			Return(powerflex.VolumeWrapper{Api: mockVolume}, nil)

		mockVolume.EXPECT().
			MapVolumeSdc(gomock.Any()).
			Return(errors.New("mapping failed"))

		err := clonner.Map(initatorGroup, targetLUN)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mapping failed")
	})
}

func TestUnMap(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock objects
	mockClient := mocks.NewMockPowerFlexClient(ctrl)
	mockVolume := mocks.NewMockPowerFlexVolume(ctrl)
	typeVolume := types.Volume{}
	mockSystem := goscaleio.System{}
	initatorGroup := "test-group"
	targetLUN := populator.LUN{SerialNumber: "12345"} // Ensure this type is correctly imported
	clonner := powerflex.PowerFlexClonner{Client: mockClient, System: mockSystem}

	// Expect GetVolume to return mockVolume inside a slice
	t.Run("unmap success", func(t *testing.T) {

		mockClient.EXPECT().
			GetVolume("", "12345", "", "", false).
			Return([]*types.Volume{&typeVolume}, nil)

		mockClient.EXPECT().
			NewVolume().
			Return(powerflex.VolumeWrapper{Api: mockVolume}, nil)

		// Expect MapVolumeSdc to be called and return nil (success)
		mockVolume.EXPECT().
			UnmapVolumeSdc(gomock.Any()).
			Return(nil)

		// Create an instance of PowerFlexClonner with the mocked client

		// Execute the function
		err := clonner.UnMap(initatorGroup, targetLUN)

		// Validate no errors occurred
		assert.NoError(t, err)
	})
	t.Run("GetVolume fails", func(t *testing.T) {
		mockClient.EXPECT().
			GetVolume("", "12345", "", "", false).
			Return(nil, errors.New("failed to get volume"))

		err := clonner.UnMap(initatorGroup, targetLUN)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get volume")
	})

	// --- Scenario 2: NewVolume fails ---
	t.Run("NewVolume fails", func(t *testing.T) {
		mockClient.EXPECT().
			GetVolume("", "12345", "", "", false).
			Return([]*types.Volume{&typeVolume}, nil)

		mockClient.EXPECT().
			NewVolume().
			Return(powerflex.VolumeWrapper{Api: nil}, errors.New("failed to create volume service"))

		err := clonner.UnMap(initatorGroup, targetLUN)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create volume service")
	})

	// --- Scenario 3: MapVolumeSdc fails ---
	t.Run("MapVolumeSdc fails", func(t *testing.T) {
		mockClient.EXPECT().
			GetVolume("", "12345", "", "", false).
			Return([]*types.Volume{&typeVolume}, nil)

		mockClient.EXPECT().
			NewVolume().
			Return(powerflex.VolumeWrapper{Api: mockVolume}, nil)

		mockVolume.EXPECT().
			UnmapVolumeSdc(gomock.Any()).
			Return(errors.New("mapping failed"))

		err := clonner.UnMap(initatorGroup, targetLUN)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mapping failed")
	})
}

func TestResolveVolumeHandleToLUN(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cloner := &powerflex.PowerFlexClonner{}

	t.Run("ResolvePVToSystemId success", func(t *testing.T) {
		volumeHandle := "csi-12345"
		lun, err := cloner.ResolveVolumeHandleToLUN(volumeHandle)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		expectedLUN := populator.LUN{Name: "12345", SerialNumber: "12345"}
		if lun != expectedLUN {
			t.Errorf("expected %v, got %v", expectedLUN, lun)
		}
	})

	t.Run("ResolvePVToSystemId fails due to incorrect VolumeHandle format", func(t *testing.T) {
		volumeHandle := "csi12345"
		_, err := cloner.ResolveVolumeHandleToLUN(volumeHandle)
		if err == nil {
			t.Errorf("expected error but got nil")
		}
	})
}
func TestCurrentMappedGroups(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockPowerFlexClient(ctrl)
	mockVolume := mocks.NewMockPowerFlexVolume(ctrl)
	clonner := powerflex.PowerFlexClonner{Client: mockClient}

	t.Run("happy path", func(t *testing.T) {
		mappedSdcInfo := types.MappedSdcInfo{SdcID: "12345"}
		mappedSdcInfoSlice := []*types.MappedSdcInfo{&mappedSdcInfo}
		typeVolume := types.Volume{MappedSdcInfo: mappedSdcInfoSlice}
		targetLUN := populator.LUN{Name: "12345", SerialNumber: "12345"}
		mockClient.EXPECT().
			GetVolume("", "12345", "", "", false).
			Return([]*types.Volume{&typeVolume}, nil)

		mockClient.EXPECT().
			NewVolume().
			Return(powerflex.VolumeWrapper{Api: mockVolume, Volume: &typeVolume}, nil)
		mappedSdcIds, err := clonner.CurrentMappedGroups(targetLUN)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		ids := []string{"12345"}
		assert.Equal(t, mappedSdcIds, ids)
	})
	t.Run("no sdc mapped", func(t *testing.T) {
		mappedSdcInfoSlice := []*types.MappedSdcInfo{}
		typeVolume := types.Volume{MappedSdcInfo: mappedSdcInfoSlice}
		targetLUN := populator.LUN{Name: "12345", SerialNumber: "12345"}
		mockClient.EXPECT().
			GetVolume("", "12345", "", "", false).
			Return([]*types.Volume{&typeVolume}, nil)

		mockClient.EXPECT().
			NewVolume().
			Return(powerflex.VolumeWrapper{Api: mockVolume, Volume: &typeVolume}, nil)
		mappedSdcIds, err := clonner.CurrentMappedGroups(targetLUN)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if mappedSdcIds != nil {
			t.Errorf("expected nil, got %v", mappedSdcIds)
		}
	})
}
