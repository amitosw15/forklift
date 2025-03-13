package main

import (
	"context"
	"fmt"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"log"
	"net/url"
	"os"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/types"
)

func main() {
	vcURL := os.Getenv("GOVMOMI_URL")
	user := os.Getenv("GOVMOMI_USERNAME")
	pass := os.Getenv("GOVMOMI_PASSWORD")
	//datastoreName := "eco-iscsi-ds1" // Extracted from 3par's info
	vmName := "3par"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to vCenter
	u, err := url.Parse(vcURL)
	if err != nil {
		log.Fatal("Error parsing vCenter URL:", err)
	}
	u.User = url.UserPassword(user, pass)
	c, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		log.Fatal("Failed to connect to vCenter:", err)
	}
	defer c.Logout(ctx)

	// Find datacenter
	finder := find.NewFinder(c.Client, true)

	//createSingleVm(ctx, c, finder, vmName, datastoreName)
	queryVM(ctx, c, finder, vmName)
}

func queryVM(ctx context.Context, c *govmomi.Client, finder *find.Finder, vmName string) {
	dc, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		log.Fatal("Failed to find default datacenter:", err)
	}
	finder.SetDatacenter(dc)

	// Find the VM by name
	vm, err := finder.VirtualMachine(ctx, vmName)
	if err != nil {
		log.Fatal("Failed to find VM:", err)
	}

	// Retrieve VM properties
	var vmProps mo.VirtualMachine
	err = vm.Properties(ctx, vm.Reference(), []string{"summary", "config.hardware.device", "resourcePool"}, &vmProps)
	if err != nil {
		log.Fatal("Failed to retrieve VM properties:", err)
	}
	// Get the resource pool reference
	resourcePoolRef := vmProps.ResourcePool
	if resourcePoolRef == nil {
		log.Fatal("VM does not belong to any resource pool")
	}

	// Convert the resource pool reference into a govmomi object
	rp := object.NewResourcePool(c.Client, *resourcePoolRef)

	// Get the resource pool name

	rpName, err := rp.ObjectName(ctx)
	if err != nil {
		log.Fatal("Failed to get resource pool name:", err)
	}

	// Print VM details
	fmt.Println("\n--- VM Information ---")
	fmt.Printf("Name: %s\n", vmProps.Summary.Config.Name)
	fmt.Printf("Guest OS: %s\n", vmProps.Summary.Config.GuestId)
	fmt.Printf("CPU: %d\n", vmProps.Summary.Config.NumCpu)
	fmt.Printf("Memory: %d MB\n", vmProps.Summary.Config.MemorySizeMB)
	fmt.Printf("Power State: %s\n", vmProps.Summary.Runtime.PowerState)
	fmt.Printf("VM Path: %s\n", vmProps.Summary.Config.VmPathName)
	fmt.Printf("VM: %s is in Resource Pool: %s\n", vmName, rpName)

	// Print network details
	fmt.Println("\n--- Network Information ---")
	for _, device := range vmProps.Config.Hardware.Device {
		if nic, ok := device.(*types.VirtualEthernetCard); ok {
			fmt.Printf("Network Adapter: %s\n", nic.DeviceInfo.GetDescription().Summary)
		}
	}

	fmt.Println("\n--- Verification Complete ---")
}

func createSingleVm(ctx context.Context, c *govmomi.Client, finder *find.Finder, vmName, datastoreName string) error {
	// Find datacenter
	dc, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		return fmt.Errorf("failed to find default datacenter: %w", err)
	}
	finder.SetDatacenter(dc)

	// Find VM folder
	folder, err := finder.Folder(ctx, "vm")
	if err != nil {
		return fmt.Errorf("failed to find VM folder: %w", err)
	}

	// Find resource pool
	pool, err := finder.DefaultResourcePool(ctx)
	if err != nil {
		return fmt.Errorf("failed to find default resource pool: %w", err)
	}

	// Find datastore
	datastore, err := finder.Datastore(ctx, datastoreName)
	if err != nil {
		return fmt.Errorf("failed to find datastore: %w", err)
	}

	// Define VM configuration
	spec := types.VirtualMachineConfigSpec{
		Name:     vmName,
		GuestId:  "fedoraGuest",
		MemoryMB: 2048,
		NumCPUs:  2,
		Files: &types.VirtualMachineFileInfo{
			VmPathName: fmt.Sprintf("[%s]", datastore.Name()),
		},
	}

	// Create the VM
	task, err := folder.CreateVM(ctx, spec, pool, nil)
	if err != nil {
		return fmt.Errorf("failed to create VM: %w", err)
	}

	// Wait for task completion
	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		return fmt.Errorf("VM creation task failed: %w", err)
	}

	fmt.Println("VM successfully created:", vmName)
	return nil
}
