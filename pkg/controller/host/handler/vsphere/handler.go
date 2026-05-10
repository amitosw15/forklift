package vsphere

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/vsphere"
	"github.com/kubev2v/forklift/pkg/controller/watch/handler"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	libweb "github.com/kubev2v/forklift/pkg/lib/inventory/web"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// Package logger.
var log = logging.WithName("host|vsphere")

var k8sNameUnsafe = regexp.MustCompile(`[^a-z0-9-]`)

func sanitizeK8sName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = k8sNameUnsafe.ReplaceAllString(s, "")
	s = strings.Trim(s, "-")
	if len(s) > 253 {
		s = s[:253]
	}
	if s == "" {
		s = "host"
	}
	return s
}

// Provider watch event handler.
type Handler struct {
	*handler.Handler
}

// Ensure watch on hosts.
func (r *Handler) Watch(watch *handler.WatchManager) (err error) {
	w, err := watch.Ensure(
		r.Provider(),
		&vsphere.Host{},
		r)
	if err != nil {
		return
	}

	log.Info(
		"Inventory watch ensured.",
		"provider",
		path.Join(
			r.Provider().Namespace,
			r.Provider().Name),
		"watch",
		w.ID())

	r.ensureHostCRs()

	return
}

// Resource created.
func (r *Handler) Created(e libweb.Event) {
	if host, cast := e.Resource.(*vsphere.Host); cast {
		r.ensureHostCR(host)
		r.changed(host)
	}
}

// Resource created.
func (r *Handler) Updated(e libweb.Event) {
	if host, cast := e.Resource.(*vsphere.Host); cast {
		updated := e.Updated.(*vsphere.Host)
		if updated.Path != host.Path || updated.InMaintenanceMode != host.InMaintenanceMode {
			r.changed(host, updated)
		}
	}
}

// Resource deleted.
func (r *Handler) Deleted(e libweb.Event) {
	if host, cast := e.Resource.(*vsphere.Host); cast {
		r.changed(host)
	}
}

// Host changed.
// Find all of the HostMap CRs the reference both the
// provider and the changed host and enqueue reconcile events.
func (r *Handler) changed(models ...*vsphere.Host) {
	log.V(3).Info(
		"Host changed.",
		"id",
		models[0].ID)
	list := api.HostList{}
	err := r.List(context.TODO(), &list)
	if err != nil {
		err = liberr.Wrap(err)
		log.Error(err, "failed to list Host CRs")
		return
	}
	for i := range list.Items {
		h := &list.Items[i]
		if !r.MatchProvider(h.Spec.Provider) {
			continue
		}
		referenced := false
		ref := h.Spec.Ref
		for _, host := range models {
			if ref.ID == host.ID || strings.HasSuffix(host.Path, ref.Name) {
				referenced = true
				break
			}
		}
		if referenced {
			log.V(3).Info(
				"Queue reconcile event.",
				"host",
				path.Join(
					h.Namespace,
					h.Name))
			r.Enqueue(event.GenericEvent{
				Object: h,
			})
		}
	}
}

// ensureHostCRs creates Host CRs for all inventory hosts that don't have one yet.
// Called from Watch() for initial population when the Provider becomes Ready.
func (r *Handler) ensureHostCRs() {
	provider := r.Provider()
	if !provider.UseVIBMethod() {
		return
	}

	var inventoryHosts []vsphere.Host
	err := r.Inventory().List(&inventoryHosts)
	if err != nil {
		log.Error(liberr.Wrap(err), "Failed to list inventory hosts for Host CR creation")
		return
	}

	if len(inventoryHosts) == 0 {
		return
	}

	existingHosts := &api.HostList{}
	err = r.List(context.TODO(), existingHosts, client.InNamespace(provider.Namespace))
	if err != nil {
		log.Error(liberr.Wrap(err), "Failed to list existing Host CRs")
		return
	}
	existingByID := map[string]bool{}
	for i := range existingHosts.Items {
		h := &existingHosts.Items[i]
		if r.MatchProvider(h.Spec.Provider) {
			existingByID[h.Spec.ID] = true
		}
	}

	for i := range inventoryHosts {
		invHost := &inventoryHosts[i]
		if existingByID[invHost.ID] {
			continue
		}
		r.createHostCR(invHost)
	}
}

// ensureHostCR creates a Host CR for a single inventory host if one doesn't exist.
// Called from Created() when a new host appears in inventory.
func (r *Handler) ensureHostCR(invHost *vsphere.Host) {
	provider := r.Provider()
	if !provider.UseVIBMethod() {
		return
	}

	existingHosts := &api.HostList{}
	err := r.List(context.TODO(), existingHosts, client.InNamespace(provider.Namespace))
	if err != nil {
		log.Error(liberr.Wrap(err), "Failed to list existing Host CRs")
		return
	}
	for i := range existingHosts.Items {
		h := &existingHosts.Items[i]
		if r.MatchProvider(h.Spec.Provider) && h.Spec.ID == invHost.ID {
			return
		}
	}

	r.createHostCR(invHost)
}

// createHostCR constructs and creates a single Host CR owned by the Provider.
func (r *Handler) createHostCR(invHost *vsphere.Host) {
	provider := r.Provider()

	hostName := fmt.Sprintf("%s-%s", provider.Name, sanitizeK8sName(invHost.Name))
	if len(hostName) > 253 {
		hostName = hostName[:253]
	}

	hostCR := &api.Host{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hostName,
			Namespace: provider.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         api.SchemeGroupVersion.String(),
					Kind:               "Provider",
					Name:               provider.Name,
					UID:                provider.UID,
					Controller:         &[]bool{true}[0],
					BlockOwnerDeletion: &[]bool{true}[0],
				},
			},
		},
		Spec: api.HostSpec{
			Ref:       refapi.Ref{ID: invHost.ID, Name: invHost.Name},
			Provider:  core.ObjectReference{Name: provider.Name, Namespace: provider.Namespace},
			IpAddress: invHost.ManagementServerIp,
		},
	}

	err := r.Create(context.TODO(), hostCR)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return
		}
		log.Error(err, "Failed to create Host CR", "host", hostName)
		return
	}

	log.Info("Auto-created Host CR", "host", hostName, "esxiHost", invHost.Name)
	r.Enqueue(event.GenericEvent{Object: hostCR})
}
