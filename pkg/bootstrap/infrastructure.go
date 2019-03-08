package bootstrap

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash/fnv"

	azhelpers "github.com/awesomenix/azkube/pkg/azure"
	"github.com/awesomenix/azkube/pkg/helpers"
)

const (
	azkubeLoadBalancerName         = "azkube-lb"
	azkubeInternalLoadBalancerName = "azkube-internal-lb"
	azkubePublicIPName             = "azkube-publicip"
	masterAvailabilitySetName      = "azkube-masters-availabilityset"
)

func (spec *Spec) preRequisites(kubernetesVersion string) string {
	return fmt.Sprintf(`
%[1]s
sudo cp -f /etc/hosts /tmp/hostsupdate
sudo chown $(id -u):$(id -g) /tmp/hostsupdate
echo '192.0.0.4 %[2]s' >> /tmp/hostsupdate
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
bootstrapTokens:
- groups:
  - system:bootstrappers:kubeadm:default-node-token
  token: %[1]s
  ttl: 48h0m0s
  usages:
  - signing
  - authentication
kind: InitConfiguration
---
apiVersion: kubeadm.k8s.io/v1beta1
kind: ClusterConfiguration
apiServer:
  certSANs:
  - "%[2]s"
  - "%[3]s"
  - "192.0.0.100"
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
kubernetesVersion: %[4]s
controlPlaneEndpoint: "%[3]s:6443"
networking:
  podSubnet: "192.168.0.0/16"
EOF
`, spec.BootstrapToken,
		spec.PublicDNSName,
		spec.InternalDNSName,
		kubernetesVersion)
}

func (spec *Spec) GetEncodedPrimaryMasterStartupScript(kubernetesVersion string) string {
	return base64.StdEncoding.EncodeToString([]byte(spec.GetPrimaryMasterStartupScript(kubernetesVersion)))
}

func (spec *Spec) GetPrimaryMasterStartupScript(kubernetesVersion string) string {
	return fmt.Sprintf(`
%[1]s
%[2]s
sudo kubeadm init --config /tmp/kubeadm-config.yaml
mkdir -p $HOME/.kube
sudo cp -f /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
%[3]s
`, spec.kubeadmInitConfig(kubernetesVersion),
		spec.preRequisites(kubernetesVersion),
		helpers.CalicoCNI())
}

func (spec *Spec) CreateBaseInfrastructure() error {
	log.Info("Creating", "ResourceGroup", spec.GroupName, "Location", spec.GroupLocation)
	err := spec.CreateOrUpdateResourceGroup(context.TODO())
	if err != nil {
		return err
	}
	log.Info("Successfully Created", "ResourceGroup", spec.GroupName, "Location", spec.GroupLocation)

	log.Info("Creating", "VNET", "azkube-vnet", "Location", spec.GroupLocation)
	err = spec.CreateVirtualNetworkAndSubnets(context.TODO(), "azkube-vnet")
	if err != nil {
		return err
	}
	log.Info("Successfully Created", "VNET", "azkube-vnet", "Location", spec.GroupLocation)

	log.Info("Creating", "AvailabilitySet", masterAvailabilitySetName)
	if _, err := spec.CreateAvailabilitySet(
		context.TODO(),
		masterAvailabilitySetName); err != nil {
		return err
	}
	log.Info("Successfully Created", "AvailabilitySet", masterAvailabilitySetName)

	log.Info("Creating Internal Load Balancer", "Name", azkubeInternalLoadBalancerName)
	if err := spec.CreateInternalLoadBalancer(
		context.TODO(),
		"azkube-vnet",
		"master-subnet",
		azkubeInternalLoadBalancerName); err != nil {
		return err
	}
	log.Info("Successfully Created Internal Load Balancer", "Name", azkubeInternalLoadBalancerName)

	h := fnv.New32a()
	h.Write([]byte(fmt.Sprintf("%s-%s", azkubePublicIPName, spec.ClusterName)))
	publicIPName := fmt.Sprintf("%x", h.Sum32())
	publicIPName = spec.DNSPrefix + publicIPName

	log.Info("Creating Public Load Balancer", "Name", azkubeLoadBalancerName, "PublicIPName", publicIPName)
	if err := spec.CreateLoadBalancer(
		context.TODO(),
		azkubeLoadBalancerName,
		publicIPName); err != nil {
		return err
	}
	log.Info("Successfully Created Public Load Balancer", "Name", azkubeLoadBalancerName, "PublicIPName", publicIPName)

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
		"/etc/kubernetes/init-azure-bootstrap.sh": spec.GetPrimaryMasterStartupScript(spec.BootstrapKubernetesVersion),
	}

	vmName := fmt.Sprintf("%s-mastervm-0", spec.ClusterName)
	log.Info("Creating", "VM", vmName)
	if err := spec.CreateVMWithLoadBalancer(
		context.TODO(),
		vmName,
		"azkube-lb",
		"azkube-internal-lb",
		"azkube-vnet",
		"master-subnet",
		fmt.Sprintf("192.0.0.4"),
		base64.StdEncoding.EncodeToString([]byte(azhelpers.GetCustomData(customData, customRunData))),
		masterAvailabilitySetName,
		vmSKUType,
		0); err != nil {
		log.Error(err, "Creation Failed", "VM", vmName)
		return err
	}
	log.Info("Successfully Created", "VM", vmName)

	return nil
}

func (spec *Spec) CleanupInfrastructure() error {
	return spec.DeleteResourceGroup(context.TODO())
}
