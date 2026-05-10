package host

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	vspherelib "github.com/kubev2v/forklift/pkg/lib/client/vsphere"
	"github.com/kubev2v/forklift/pkg/lib/client/vsphere/vmware"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	libref "github.com/kubev2v/forklift/pkg/lib/ref"
	"github.com/kubev2v/forklift/pkg/settings"
	"golang.org/x/crypto/ssh"
	core "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultVIBPath = "/usr/local/share/forklift/vmkfstools-wrapper.vib"
	tempVIBPath    = "/tmp/vmkfstools-wrapper.vib"
	sshTimeout     = 30 * time.Second
	hostdWait      = 10 * time.Second
)

const (
	ProviderVIBReady    = "VIBReady"
	ProviderVIBNotReady = "VIBNotReady"
)

// checkAndAggregateVIB checks VIB installation on all hosts for this Provider
// via the vCenter API (using the Provider's credentials) and updates:
// - each Host CR's VIBInstalled condition (Durable)
// - the Provider's VIBReady/VIBNotReady conditions with per-host Items
//
// Called from validate() before validateSecret(), so it runs even for
// credential-less auto-created hosts.
func (r *Reconciler) checkAndAggregateVIB(host *api.Host) error {
	provider := host.Referenced.Provider.Source
	if provider == nil || provider.Type() != api.VSphere || !provider.UseVIBMethod() {
		return nil
	}

	providerSettings := &settings.Providers{}
	if err := providerSettings.Load(); err != nil {
		return err
	}

	vibCondition := host.Status.FindCondition(VIBInstalled)
	if vibCondition != nil && vspherelib.ShouldSkipVIBCheck(vibCondition.LastTransitionTime.Time, providerSettings.VIBCacheDuration) {
		cond := *vibCondition
		cond.Durable = true
		host.Status.SetCondition(cond)
		return nil
	}

	if provider.IsHost() {
		return nil
	}
	providerSecret := &core.Secret{}
	err := r.Get(context.TODO(), client.ObjectKey{
		Namespace: provider.Spec.Secret.Namespace,
		Name:      provider.Spec.Secret.Name,
	}, providerSecret)
	if err != nil {
		return liberr.Wrap(err)
	}

	vcenterURL := provider.Spec.URL
	username := string(providerSecret.Data["user"])
	password := string(providerSecret.Data["password"])

	vClient, err := vmware.NewClient(vcenterURL, username, password)
	if err != nil {
		return liberr.Wrap(err)
	}
	defer func() { _ = vClient.Logout(context.TODO()) }()

	ctx := context.TODO()
	esxHosts, err := vClient.GetAllHosts(ctx)
	if err != nil {
		return liberr.Wrap(err)
	}

	esxByID := map[string]interface{}{}
	type hostCheckResult struct {
		ready   bool
		version string
	}
	esxResults := map[string]hostCheckResult{}

	for _, esx := range esxHosts {
		id := esx.Reference().Value
		esxByID[id] = esx

		version, verr := vspherelib.GetLoadedVIBVersion(ctx, vClient, esx)
		if verr != nil {
			r.Log.V(2).Info("VIB version check failed", "host", esx.Name(), "err", verr)
		}
		ready := version == vspherelib.VibVersion
		esxResults[id] = hostCheckResult{ready: ready, version: version}
	}

	hostList := &api.HostList{}
	err = r.List(context.TODO(), hostList, client.InNamespace(host.Namespace))
	if err != nil {
		return err
	}

	providerRef := &core.ObjectReference{
		Namespace: provider.Namespace,
		Name:      provider.Name,
	}

	var readyHosts, notReadyHosts []string

	for i := range hostList.Items {
		h := &hostList.Items[i]
		hProviderRef := &h.Spec.Provider
		if !libref.Equals(hProviderRef, providerRef) {
			continue
		}

		result, found := esxResults[h.Spec.ID]
		if !found {
			notReadyHosts = append(notReadyHosts, h.Spec.Name)
			if h.Name == host.Name {
				host.Status.DeleteCondition(VIBInstalled)
			} else {
				h.Status.DeleteCondition(VIBInstalled)
				_ = r.Status().Update(context.TODO(), h)
			}
			continue
		}

		if result.ready {
			readyHosts = append(readyHosts, h.Spec.Name)
			cond := libcnd.Condition{
				Type:     VIBInstalled,
				Status:   True,
				Reason:   "Installed",
				Category: Required,
				Message:  fmt.Sprintf("VIB %s installed and active.", vspherelib.VibVersion),
				Durable:  true,
			}
			if h.Name == host.Name {
				host.Status.SetCondition(cond)
			} else {
				h.Status.SetCondition(cond)
				_ = r.Status().Update(context.TODO(), h)
			}
		} else {
			notReadyHosts = append(notReadyHosts, h.Spec.Name)
			if h.Name == host.Name {
				host.Status.DeleteCondition(VIBInstalled)
			} else {
				h.Status.DeleteCondition(VIBInstalled)
				_ = r.Status().Update(context.TODO(), h)
			}
		}
	}

	return r.updateProviderVIBStatus(host, readyHosts, notReadyHosts)
}

// updateProviderVIBStatus updates the Provider's VIBReady/VIBNotReady conditions
// with the aggregated host lists. Uses retry.RetryOnConflict to handle concurrent updates.
func (r *Reconciler) updateProviderVIBStatus(host *api.Host, readyHosts, notReadyHosts []string) error {
	provider := host.Referenced.Provider.Source
	if provider == nil {
		return nil
	}

	const maxRetries = 5
	for attempt := 0; attempt < maxRetries; attempt++ {
		p := &api.Provider{}
		err := r.Get(context.TODO(), client.ObjectKey{
			Namespace: provider.Namespace,
			Name:      provider.Name,
		}, p)
		if err != nil {
			return err
		}

		if len(readyHosts) > 0 {
			p.Status.SetCondition(libcnd.Condition{
				Type:     ProviderVIBReady,
				Status:   True,
				Reason:   "HostVIBInstalled",
				Category: Required,
				Message:  fmt.Sprintf("VIB installed on %d host(s).", len(readyHosts)),
				Items:    readyHosts,
				Durable:  true,
			})
		} else {
			p.Status.DeleteCondition(ProviderVIBReady)
		}

		if len(notReadyHosts) > 0 {
			p.Status.SetCondition(libcnd.Condition{
				Type:     ProviderVIBNotReady,
				Status:   True,
				Reason:   "HostVIBNotInstalled",
				Category: Warn,
				Message:  fmt.Sprintf("VIB not ready on %d host(s).", len(notReadyHosts)),
				Items:    notReadyHosts,
				Durable:  true,
			})
		} else {
			p.Status.DeleteCondition(ProviderVIBNotReady)
		}

		err = r.Status().Update(context.TODO(), p)
		if err == nil {
			return nil
		}
		if !k8serr.IsConflict(err) {
			return err
		}
		r.Log.V(2).Info("Provider status update conflict, retrying", "attempt", attempt+1)
	}
	return fmt.Errorf("failed to update Provider VIB status after %d retries", maxRetries)
}

// ensureVIB handles SSH-based VIB installation on a single host.
// Only runs when installVIB=true, VIB is not already installed, and SSH credentials exist.
func (r *Reconciler) ensureVIB(host *api.Host) (time.Duration, error) {
	if !host.Spec.InstallVIB {
		return 0, nil
	}

	if host.Status.HasCondition(VIBInstalled) {
		return 0, nil
	}

	secret := host.Referenced.Secret
	if secret == nil {
		return 0, nil
	}

	installed, err := installVIBOnHost(host.Spec.IpAddress, secret, r.Log)
	if err != nil {
		r.Log.Error(err, "VIB installation failed", "host", host.Spec.IpAddress)
		host.Status.DeleteCondition(VIBInstalled)
		host.Status.SetCondition(libcnd.Condition{
			Type:     VIBInstallFailed,
			Status:   True,
			Reason:   "InstallFailed",
			Category: Warn,
			Message:  fmt.Sprintf("VIB installation failed on host %s: %v", host.Spec.IpAddress, err),
			Durable:  true,
		})
		return 0, nil
	}

	if installed {
		r.Log.Info("VIB installed, requeueing to verify after hostd restart", "host", host.Spec.IpAddress)
		return hostdWait, nil
	}

	host.Status.DeleteCondition(VIBInstallFailed)
	host.Status.SetCondition(libcnd.Condition{
		Type:     VIBInstalled,
		Status:   True,
		Reason:   "Installed",
		Category: Required,
		Message:  fmt.Sprintf("VIB %s installed and active.", vspherelib.VibVersion),
		Durable:  true,
	})
	return 0, nil
}

// installVIBOnHost connects to an ESXi host via SSH and installs the VIB.
// Returns (true, nil) when a fresh install was performed and hostd was restarted —
// the caller should wait before verifying. Returns (false, nil) when the VIB
// was already at the desired version.
func installVIBOnHost(hostIP string, secret *core.Secret, log logging.LevelLogger) (bool, error) {
	user := string(secret.Data["user"])
	password := string(secret.Data["password"])

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
			ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = password
				}
				return answers, nil
			}),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         sshTimeout,
	}

	addr := fmt.Sprintf("%s:22", hostIP)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return false, fmt.Errorf("SSH connection failed: %w", err)
	}
	defer func() { _ = client.Close() }()

	log.Info("SSH connected", "host", hostIP)

	loadedVersion, err := getLoadedVersion(client)
	if err != nil {
		log.V(2).Info("VIB version check failed, proceeding with install", "host", hostIP, "err", err)
	}
	if loadedVersion == vspherelib.VibVersion {
		log.Info("VIB already at desired version", "host", hostIP, "version", loadedVersion)
		return false, nil
	}

	log.Info("Installing VIB", "host", hostIP, "current", loadedVersion, "desired", vspherelib.VibVersion)

	if err := copyFileToHost(client, DefaultVIBPath, tempVIBPath); err != nil {
		return false, fmt.Errorf("failed to copy VIB file: %w", err)
	}

	_, err = runSSHCommand(client, fmt.Sprintf("esxcli software vib install -v %s -f", tempVIBPath))
	if err != nil {
		return false, fmt.Errorf("VIB install command failed: %w", err)
	}

	_, _ = runSSHCommand(client, fmt.Sprintf("rm -f %s", tempVIBPath))

	log.Info("Restarting hostd", "host", hostIP)
	_, err = runSSHCommand(client, "/etc/init.d/hostd restart")
	if err != nil {
		return false, fmt.Errorf("hostd restart failed: %w", err)
	}

	log.Info("VIB installed, hostd restarting", "host", hostIP)
	return true, nil
}

// getLoadedVersion runs "esxcli vmkfstools version" over SSH and extracts
// the version from the "Message: X.Y.Z" line in the esxcli output.
func getLoadedVersion(client *ssh.Client) (string, error) {
	output, err := runSSHCommand(client, "esxcli vmkfstools version")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Message:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Message:")), nil
		}
	}
	return "", fmt.Errorf("no Message field in esxcli output: %s", strings.TrimSpace(output))
}

func runSSHCommand(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	return string(output), err
}

func copyFileToHost(client *ssh.Client, localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read local file %s: %w", localPath, err)
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	if err := session.Start(fmt.Sprintf("cat > %s", remotePath)); err != nil {
		return fmt.Errorf("failed to start cat command: %w", err)
	}

	if _, err := stdin.Write(data); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("failed to write data: %w", err)
	}
	_ = stdin.Close()

	if err := session.Wait(); err != nil {
		return fmt.Errorf("file copy failed: %w", err)
	}

	return nil
}
