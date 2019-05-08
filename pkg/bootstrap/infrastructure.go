package bootstrap

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash/fnv"

	azhelpers "github.com/awesomenix/azk/pkg/azure"
	"github.com/awesomenix/azk/pkg/helpers"
)

const (
	azkLoadBalancerName         = "azk-lb"
	azkInternalLoadBalancerName = "azk-internal-lb"
	azkPublicIPName             = "azk-publicip"
	masterVmssName              = "azk-master-vmss"
)

func (spec *Spec) preRequisites(kubernetesVersion string) string {
	return fmt.Sprintf(`
%[1]s
sudo cp -f /etc/hosts /tmp/hostsupdate
sudo chown $(id -u):$(id -g) /tmp/hostsupdate
echo '10.0.0.4 %[2]s' >> /tmp/hostsupdate
sudo mv /etc/hosts /etc/hosts.bak
sudo mv /tmp/hostsupdate /etc/hosts
`, helpers.PreRequisitesInstallScript(kubernetesVersion), spec.InternalDNSName)
}

func (spec *Spec) kubeadmInitConfig(kubernetesVersion string) string {
	return fmt.Sprintf(`
cat <<EOF >/tmp/kubeadm-config.yaml
apiVersion: kubeadm.k8s.io/v1beta1
nodeRegistration:
  kubeletExtraArgs:
    cloud-provider: azure
    cloud-config: /etc/kubernetes/azure.json
kind: InitConfiguration
---
apiVersion: kubeadm.k8s.io/v1beta1
kind: ClusterConfiguration
apiServer:
  certSANs:
  - "%[1]s"
  - "%[2]s"
  - "10.0.0.100"
  extraArgs:
    cloud-config: /etc/kubernetes/azure.json
    cloud-provider: azure
  extraVolumes:
  - hostPath: /etc/kubernetes/azure.json
    mountPath: /etc/kubernetes/azure.json
    name: cloud-config
    readOnly: true
controllerManager:
  extraArgs:
    cloud-config: /etc/kubernetes/azure.json
    cloud-provider: azure
  extraVolumes:
  - hostPath: /etc/kubernetes/azure.json
    mountPath: /etc/kubernetes/azure.json
    name: cloud-config
    readOnly: true
kubernetesVersion: %[3]s
controlPlaneEndpoint: "%[2]s:6443"
networking:
  podSubnet: "10.244.0.0/16"
EOF
`, spec.PublicDNSName,
		spec.InternalDNSName,
		kubernetesVersion)
}

func (spec *Spec) GetEncodedBootstrapStartupScript(kubernetesVersion string) string {
	return base64.StdEncoding.EncodeToString([]byte(spec.GetBootstrapStartupScript(kubernetesVersion)))
}

func (spec *Spec) GetBootstrapStartupScript(kubernetesVersion string) string {
	return fmt.Sprintf(`
set -eux
%[1]s
%[2]s
sudo kubeadm init --config /tmp/kubeadm-config.yaml
mkdir -p $HOME/.kube
sudo cp -f /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
%[3]s
`, spec.kubeadmInitConfig(kubernetesVersion),
		spec.preRequisites(kubernetesVersion),
		helpers.CanalCNI())
}

func (spec *Spec) CreateBaseInfrastructure() error {
	log.Info("Creating", "ResourceGroup", spec.GroupName, "Location", spec.GroupLocation)
	err := spec.CreateOrUpdateResourceGroup(context.TODO())
	if err != nil {
		return err
	}
	log.Info("Successfully Created", "ResourceGroup", spec.GroupName, "Location", spec.GroupLocation)

	log.Info("Creating", "VNET", "azk-vnet", "Location", spec.GroupLocation)
	err = spec.CreateVirtualNetworkAndSubnets(context.TODO(), "azk-vnet")
	if err != nil {
		return err
	}
	log.Info("Successfully Created", "VNET", "azk-vnet", "Location", spec.GroupLocation)

	log.Info("Creating Internal Load Balancer", "Name", azkInternalLoadBalancerName)
	if err := spec.CreateInternalLoadBalancer(
		context.TODO(),
		"azk-vnet",
		"master-subnet",
		azkInternalLoadBalancerName); err != nil {
		return err
	}
	log.Info("Successfully Created Internal Load Balancer", "Name", azkInternalLoadBalancerName)

	h := fnv.New32a()
	h.Write([]byte(fmt.Sprintf("%s-%s", azkPublicIPName, spec.ClusterName)))
	publicIPName := fmt.Sprintf("%x", h.Sum32())
	publicIPName = spec.DNSPrefix + publicIPName

	log.Info("Creating Public Load Balancer", "Name", azkLoadBalancerName, "PublicIPName", publicIPName)
	if err := spec.CreateLoadBalancer(
		context.TODO(),
		azkLoadBalancerName,
		publicIPName); err != nil {
		return err
	}
	log.Info("Successfully Created Public Load Balancer", "Name", azkLoadBalancerName, "PublicIPName", publicIPName)

	pip, err := spec.GetPublicIP(context.TODO(), publicIPName)
	if err != nil {
		return err
	}
	log.Info("Successfully Established Public IP", "Name", publicIPName, "Address", *pip.PublicIPAddressPropertiesFormat.IPAddress)

	return nil
}

func (spec *Spec) CreateInfrastructure() error {

	vmSKUType := spec.BootstrapVMSKUType
	if vmSKUType == "" {
		vmSKUType = "Standard_DS2_v2"
	}

	customData := map[string]string{
		"/etc/kubernetes/pki/ca.crt":             spec.CACertificate,
		"/etc/kubernetes/pki/ca.key":             spec.CACertificateKey,
		"/etc/kubernetes/pki/sa.key":             spec.ServiceAccountKey,
		"/etc/kubernetes/pki/sa.pub":             spec.ServiceAccountPub,
		"/etc/kubernetes/pki/front-proxy-ca.crt": spec.FrontProxyCACertificate,
		"/etc/kubernetes/pki/front-proxy-ca.key": spec.FrontProxyCACertificateKey,
		"/etc/kubernetes/pki/etcd/ca.crt":        spec.EtcdCACertificate,
		"/etc/kubernetes/pki/etcd/ca.key":        spec.EtcdCACertificateKey,
		"/etc/kubernetes/azure.json":             spec.AzureCloudProviderConfig,
		//"/etc/kubernetes/admin.conf":             status.AdminKubeConfig,
	}

	customRunData := map[string]string{
		"/etc/kubernetes/init-azure-bootstrap.sh": spec.GetBootstrapStartupScript(spec.BootstrapKubernetesVersion),
	}

	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers", spec.SubscriptionID, spec.GroupName)

	subnetID := prefix + "/Microsoft.Network/virtualNetworks/azk-vnet/subnets/master-subnet"

	loadbalancerIDs := []string{
		prefix + "/Microsoft.Network/loadBalancers/azk-lb/backendAddressPools/master-backEndPool",
		prefix + "/Microsoft.Network/loadBalancers/azk-internal-lb/backendAddressPools/master-internal-backEndPool",
	}

	natPoolIDs := []string{
		prefix + "/Microsoft.Network/loadBalancers/azk-lb/inboundNatPools/natSSHPool",
	}

	log.Info("Creating", "VMSS", masterVmssName)
	if err := spec.CreateVMSS(
		context.TODO(),
		masterVmssName,
		subnetID,
		loadbalancerIDs,
		natPoolIDs,
		base64.StdEncoding.EncodeToString([]byte(azhelpers.GetCustomData(customData, customRunData))),
		spec.BootstrapVMSKUType,
		1); err != nil {
		return err
	}
	log.Info("Successfully Created", "VMSS", masterVmssName)

	return nil
}

func (spec *Spec) CleanupInfrastructure() error {
	return spec.DeleteResourceGroup(context.TODO())
}
