package main

import (
	"context"
	"fmt"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/guest"
	"github.com/vmware/govmomi/vim25/types"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	defaultURL = "https://cloud-images.ubuntu.com/releases/focal/release/ubuntu-20.04-server-cloudimg-amd64.vmdk"
)

func runCmd(cmdName string, args ...string) error {
	fmt.Printf("Running: %s %v\n", cmdName, args)
	cmd := exec.Command(cmdName, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func CreateTestVMWithDiskAndISO(vmdkURL, vmName, isoPath, datastore string, skipUpload bool) error {
	if vmdkURL == "" {
		vmdkURL = defaultURL
	}
	vmdkFile := filepath.Base(vmdkURL)
	vmdkRemotePath := fmt.Sprintf("%s/%s", vmName, vmdkFile)
	if !skipUpload {
		if _, err := os.Stat(vmdkFile); os.IsNotExist(err) {
			fmt.Println("Downloading VMDK:", vmdkURL)
			if err := runCmd("wget", vmdkURL); err != nil {
				return fmt.Errorf("failed to download VMDK: %w", err)
			}
		} else {
			fmt.Println("VMDK already exists locally, skipping download.")
		}
		if err := runCmd("govc", "import.vmdk", "-ds="+datastore, "-pool=Resources", vmdkFile, vmName); err != nil {
			return fmt.Errorf("failed to upload VMDK: %w", err)
		}
	}

	if err := runCmd("govc", "vm.create",
		"-ds="+datastore,
		"-g=ubuntu64Guest",
		"-m=2048",
		"-c=2",
		"-net=VM Network",
		"-on=false",
		vmName,
	); err != nil {
		return fmt.Errorf("failed to create VM: %w", err)
	}

	if err := runCmd("govc", "vm.disk.attach",
		"-vm="+vmName,
		"-disk="+vmdkRemotePath,
		"-ds="+datastore,
	); err != nil {
		return fmt.Errorf("failed to attach disk: %w", err)
	}

	if err := runCmd("govc", "device.cdrom.add", "-vm="+vmName); err != nil {
		return fmt.Errorf("failed to add cdrom: %w", err)
	}

	if err := runCmd("govc", "device.cdrom.insert",
		"-vm="+vmName,
		"-device=cdrom-3000",
		isoPath,
	); err != nil {
		return fmt.Errorf("failed to insert seed ISO: %w", err)
	}

	if err := runCmd("govc", "device.connect", "-vm="+vmName, "cdrom-3000"); err != nil {
		return fmt.Errorf("failed to connect cdrom: %w", err)
	}

	if err := runCmd("govc", "vm.power", "-on", vmName); err != nil {
		return fmt.Errorf("failed to power on vm: %w", err)
	}

	fmt.Println("VM deployed and running!")
	return nil
}

func main() {
	//vcURL := os.Getenv("GOVC_URL")
	//user := os.Getenv("GOVC_USERNAME")
	//pass := os.Getenv("GOVC_PASSWORD")
	//////datastoreName := "eco-iscsi-ds1" // Extracted from 3par's info
	//vmName := "ubuntu-automate-test2"
	//ctx, cancel := context.WithCancel(context.Background())
	//defer cancel()
	//
	//// Connect to vCenter
	//u, err := url.Parse(vcURL)
	//if err != nil {
	//	log.Fatal("Error parsing vCenter URL:", err)
	//}
	//u.User = url.UserPassword(user, pass)
	//c, err := govmomi.NewClient(ctx, u, true)
	//if err != nil {
	//	log.Fatal("Failed to connect to vCenter:", err)
	//}
	//defer c.Logout(ctx)
	//
	//// Find datacenter
	//finder := find.NewFinder(c.Client, true)
	//dc, err := finder.DefaultDatacenter(ctx)
	//if err != nil {
	//	log.Fatal("Failed to find default datacenter:", err)
	//}
	//finder.SetDatacenter(dc)
	////err = CreateTestVMWithDiskAndISO("", vmName, "automatic-vm-creation-test/seed2.iso", "eco-iscsi-ds1", false)
	////if err != nil {
	////	log.Fatal(err)
	////}
	//err = changeFileSystem(ctx, c, finder, vmName, "fedora", "password", 1)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//setup(false)

}

func changeFileSystem(ctx context.Context, c *govmomi.Client, finder *find.Finder, vmName, guestUser, guestPass string, sizeMB int) error {
	fmt.Println("Changing filesystem")
	vm, err := finder.VirtualMachine(ctx, vmName)
	if err != nil {
		fmt.Println("Failed to find VM:", err)
		return fmt.Errorf("failed to find VM: %w", err)
	}
	guestOpsMgr := guest.NewOperationsManager(c.Client, vm.Reference())
	// Guest authentication
	auth := &types.NamePasswordAuthentication{
		Username: guestUser,
		Password: guestPass,
	}
	fmt.Println("Creating process....")
	procManager, err := guestOpsMgr.ProcessManager(ctx)
	var programSpec types.GuestProgramSpec
	filePath := fmt.Sprintf("/tmp/vm-%s-%s.xcopy", vmName, guestUser)
	command := fmt.Sprintf("-c 'touch %s && dd if=/dev/urandom of=%s bs=1M count=%d 2>&1'", filePath, filePath, sizeMB)
	fmt.Println(command)
	programSpec = types.GuestProgramSpec{
		ProgramPath: "/bin/sh",
		Arguments:   command,
	}
	// Execute command inside the guest
	pid, err := procManager.StartProgram(ctx, auth, &programSpec)
	if err != nil {
		fmt.Println("Failed to start program:", err)
		return fmt.Errorf("failed to start guest process: %w", err)
	}
	log.Printf("Started process inside VM with PID: %d", pid)
	return nil
}
