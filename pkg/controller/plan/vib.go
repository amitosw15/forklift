package plan

import (
	"fmt"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	"github.com/kubev2v/forklift/pkg/controller/provider/web"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/vsphere"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
)

const (
	VIBReady    = "VIBReady"
	VIBNotReady = "VIBNotReady"
)

func (r *Reconciler) validateVIBReadiness(plan *api.Plan) error {
	sourceProvider := plan.Referenced.Provider.Source
	if sourceProvider == nil {
		return nil
	}

	if sourceProvider.Type() != api.VSphere {
		return nil
	}

	if !r.planUsesVSphereXcopyPopulator(plan) {
		plan.Status.DeleteCondition(VIBReady)
		plan.Status.DeleteCondition(VIBNotReady)
		return nil
	}

	if !sourceProvider.UseVIBMethod() {
		plan.Status.DeleteCondition(VIBReady)
		plan.Status.DeleteCondition(VIBNotReady)
		return nil
	}

	vibReadyCond := sourceProvider.Status.FindCondition(VIBReady)
	readyHostNames := map[string]bool{}
	if vibReadyCond != nil {
		for _, name := range vibReadyCond.Items {
			readyHostNames[name] = true
		}
	}

	inventory, err := web.NewClient(sourceProvider)
	if err != nil {
		return err
	}

	var notReadyHosts []string
	checkedHosts := map[string]bool{}

	for i := range plan.Spec.VMs {
		vm := &plan.Spec.VMs[i]
		v, err := inventory.VM(&vm.Ref)
		if err != nil {
			continue
		}
		vsphereVM, ok := v.(*vsphere.VM)
		if !ok {
			continue
		}

		hostID := vsphereVM.Host
		if checkedHosts[hostID] {
			continue
		}
		checkedHosts[hostID] = true

		hostName := hostID
		h, hErr := inventory.Host(&refapi.Ref{ID: hostID})
		if hErr == nil {
			if vsHost, ok := h.(*vsphere.Host); ok {
				hostName = vsHost.Name
			}
		}

		if !readyHostNames[hostName] {
			notReadyHosts = append(notReadyHosts, hostName)
		}
	}

	if len(notReadyHosts) > 0 {
		plan.Status.SetCondition(libcnd.Condition{
			Type:     VIBNotReady,
			Status:   libcnd.True,
			Reason:   "HostVIBNotInstalled",
			Category: libcnd.Critical,
			Message:  fmt.Sprintf("VIB not installed on %d host(s) required by this plan.", len(notReadyHosts)),
			Items:    notReadyHosts,
		})
		plan.Status.DeleteCondition(VIBReady)
	} else {
		plan.Status.DeleteCondition(VIBNotReady)
		plan.Status.SetCondition(libcnd.Condition{
			Type:     VIBReady,
			Status:   libcnd.True,
			Reason:   "AllHostsReady",
			Category: libcnd.Required,
			Message:  "VIB installed on all hosts required by this plan.",
		})
	}

	return nil
}
