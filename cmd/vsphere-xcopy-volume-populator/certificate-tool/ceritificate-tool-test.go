package main

import (
	"context"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/simulator"
)

// TestCreateSingleVm uses the vSphere simulator to mock VM creation
func TestCreateSingleVm(t *testing.T) {
	// Start a simulated vSphere server
	ctx := context.Background()
	model := simulator.VPX()
	defer model.Remove()

	err := model.Create()
	if err != nil {
		t.Fatalf("Failed to create simulator: %v", err)
	}
	s := model.Service.NewServer()
	c, err := govmomi.NewClient(ctx, s.URL, true)

	// Create a govmomi client pointing to the simulator
	if err != nil {
		t.Fatalf("Failed to create vSphere client: %v", err)
	}

	// Finder for mock vSphere objects
	finder := find.NewFinder(c.Client, true)

	// Call the same function used in production, but with the mock client
	vmName := "test-vm"
	datastoreName := "datastore1"
	err = createSingleVm(ctx, c, finder, vmName, datastoreName)
	assert.NoError(t, err, "Failed to create VM in simulator")

	// Verify that the VM exists in the mock vSphere
	_, err = finder.VirtualMachine(ctx, vmName)
	assert.NoError(t, err, "VM not found after creation")

	// If we reach here, the test passed
	log.Println("--- VM Creation Test Passed ---")
}
