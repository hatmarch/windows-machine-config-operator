package azure

import (
	"context"
	"encoding/json"
	"fmt"

	config "github.com/openshift/api/config/v1"
	mapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	azureprovider "sigs.k8s.io/cluster-api-provider-azure/pkg/apis/azureprovider/v1beta1"

	"github.com/openshift/windows-machine-config-operator/test/e2e/clusterinfo"
)

const (
	defaultCredentialsSecretName = "azure-cloud-credentials"
	defaultImageOffer            = "WindowsServer"
	defaultImagePublisher        = "MicrosoftWindowsServer"
	defaultImageSKU              = "2019-Datacenter"
	defaultImageVersion          = "latest"
	defaultOSDiskSizeGB          = 128
	defaultStorageAccountType    = "Premium_LRS"
	defaultVMSize                = "Standard_D4s_V3"
)

// Provider is a provider struct for testing Azure
type Provider struct {
	oc     *clusterinfo.OpenShift
	vmSize string
}

// New returns a new Azure provider struct with the give client set and ssh key pair
func New(oc *clusterinfo.OpenShift, hasCustomVXLANPort bool) (*Provider, error) {
	if hasCustomVXLANPort == true {
		return nil, fmt.Errorf("custom VXLAN port is not supported on current Azure image")
	}

	return &Provider{
		oc,
		defaultVMSize,
	}, nil
}

func newAzureMachineProviderSpec(clusterID string, status *config.PlatformStatus, location, zone, vmSize string) (*azureprovider.AzureMachineProviderSpec, error) {
	if clusterID == "" {
		return nil, fmt.Errorf("clusterID is empty")
	}
	if status == nil || status == (&config.PlatformStatus{}) {
		return nil, fmt.Errorf("platform status is nil")
	}
	if status.Azure == nil || status.Azure == (&config.AzurePlatformStatus{}) {
		return nil, fmt.Errorf("azure platform status is nil")
	}
	if status.Azure.NetworkResourceGroupName == "" {
		return nil, fmt.Errorf("azure network resource group name is empty")
	}
	rg := status.Azure.ResourceGroupName
	netrg := status.Azure.NetworkResourceGroupName

	return &azureprovider.AzureMachineProviderSpec{
		UserDataSecret: &core.SecretReference{
			Name: clusterinfo.UserDataSecretName,
		},
		CredentialsSecret: &core.SecretReference{
			Name:      defaultCredentialsSecretName,
			Namespace: clusterinfo.MachineAPINamespace,
		},
		Location: location,
		Zone:     &zone,
		VMSize:   vmSize,
		Image: azureprovider.Image{
			Publisher: defaultImagePublisher,
			Offer:     defaultImageOffer,
			SKU:       defaultImageSKU,
			Version:   defaultImageVersion,
		},
		OSDisk: azureprovider.OSDisk{
			OSType:     "Windows",
			DiskSizeGB: defaultOSDiskSizeGB,
			ManagedDisk: azureprovider.ManagedDisk{
				StorageAccountType: defaultStorageAccountType,
			},
		},
		PublicIP:             false,
		Subnet:               fmt.Sprintf("%s-worker-subnet", clusterID),
		ManagedIdentity:      fmt.Sprintf("%s-identity", clusterID),
		Vnet:                 fmt.Sprintf("%s-worker-vnet", clusterID),
		ResourceGroup:        rg,
		NetworkResourceGroup: netrg,
	}, nil
}

// GenerateMachineSet generates the machineset object which is aws provider specific
func (p *Provider) GenerateMachineSet(withWindowsLabel bool, replicas int32) (*mapi.MachineSet, error) {
	clusterID, err := p.oc.GetClusterID()
	if err != nil {
		return nil, fmt.Errorf("unable to get cluster id: %v", err)
	}
	status, err := p.oc.GetPlatformStatus()
	if err != nil {
		return nil, fmt.Errorf("unable to get azure platform status: %v", err)
	}

	// Inspect master-0 to get Azure Location and Zone
	machines, err := p.oc.MachineClient.Machines("openshift-machine-api").Get(context.TODO(), clusterID+"-master-0", meta.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get master-0 machine resource: %v", err)
	}
	masterProviderSpec := new(azureprovider.AzureMachineProviderSpec)
	err = json.Unmarshal(machines.Spec.ProviderSpec.Value.Raw, masterProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal master-0 azure machine provider spec: %v", err)
	}

	// create new machine provider spec for deploying Windows node in the same Location and Zone as master-0
	providerSpec, err := newAzureMachineProviderSpec(clusterID, status, masterProviderSpec.Location, *masterProviderSpec.Zone, p.vmSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create new azure machine provider spec: %v", err)
	}

	rawProviderSpec, err := json.Marshal(providerSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal azure machine provider spec: %v", err)
	}

	matchLabels := map[string]string{
		mapi.MachineClusterIDLabel: clusterID,
	}

	machineSetName := "e2e-wmco-azure-machineset"
	if withWindowsLabel {
		matchLabels[clusterinfo.MachineOSIDLabel] = "Windows"
		machineSetName = machineSetName + "-with-windows-label"
	}
	matchLabels[clusterinfo.MachineSetLabel] = machineSetName

	machineLabels := map[string]string{
		clusterinfo.MachineRoleLabel: "worker",
		clusterinfo.MachineTypeLabel: "worker",
	}

	// Set up the test machineSet
	machineSet := &mapi.MachineSet{
		ObjectMeta: meta.ObjectMeta{
			Name:      machineSetName + rand.String(4),
			Namespace: clusterinfo.MachineAPINamespace,
			Labels: map[string]string{
				mapi.MachineClusterIDLabel: clusterID,
			},
		},
		Spec: mapi.MachineSetSpec{
			Selector: meta.LabelSelector{
				MatchLabels: matchLabels,
			},
			Replicas: &replicas,
			Template: mapi.MachineTemplateSpec{
				ObjectMeta: mapi.ObjectMeta{Labels: machineLabels},
				Spec: mapi.MachineSpec{
					ObjectMeta: mapi.ObjectMeta{
						Labels: map[string]string{
							"node-role.kubernetes.io/worker": "",
						},
					},
					ProviderSpec: mapi.ProviderSpec{
						Value: &runtime.RawExtension{
							Raw: rawProviderSpec,
						},
					},
				},
			},
		},
	}
	return machineSet, nil
}
