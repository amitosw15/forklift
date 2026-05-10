package populator

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
)

func getHostDC(esx *object.HostSystem) (*object.Datacenter, error) {
	ctx := context.Background()
	hostRef := esx.Reference()
	pc := property.DefaultCollector(esx.Client())
	var hostMo mo.HostSystem
	err := pc.RetrieveOne(context.Background(), hostRef, []string{"parent"}, &hostMo)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve host parent: %w", err)
	}

	parentRef := hostMo.Parent
	currentParentRef := parentRef

	for {
		if currentParentRef.Type == "Datacenter" {
			finder := find.NewFinder(esx.Client(), true)
			datacenter, err := finder.Datacenter(ctx, currentParentRef.String())
			if err != nil {
				return nil, err
			}
			return datacenter, nil
		}

		var genericParentMo mo.ManagedEntity
		err = pc.RetrieveOne(context.Background(), *currentParentRef, []string{"parent"}, &genericParentMo)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve intermediate parent: %w", err)
		}

		if genericParentMo.Parent == nil {
			break
		}
		currentParentRef = genericParentMo.Parent
	}

	return nil, fmt.Errorf("could not determine datacenter for host '%s'", esx.Name())
}
