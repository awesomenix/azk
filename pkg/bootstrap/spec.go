package bootstrap

import (
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"os"
	"strings"
	"time"

	azhelpers "github.com/awesomenix/azk/pkg/azure"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmscheme "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/scheme"
	kubeadmv1beta1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	tokenphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/bootstraptoken/node"
	kubeconfigphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	tmpDir = "/tmp/kubeadm"
)

type Spec struct {
	azhelpers.CloudConfiguration `json:"inline"`
	DNSPrefix                    string   `json:"dnsPrefix,omitempty"`
	ClusterName                  string   `json:"clusterName,omitempty"`
	CACertificate                string   `json:"caCertificate,omitempty"`
	CACertificateKey             string   `json:"caCertificateKey,omitempty"`
	ServiceAccountKey            string   `json:"serviceAccountKey,omitempty"`
	ServiceAccountPub            string   `json:"serviceAccountPub,omitempty"`
	FrontProxyCACertificate      string   `json:"frontProxyCACertificate,omitempty"`
	FrontProxyCACertificateKey   string   `json:"frontProxyCACertificateKey,omitempty"`
	EtcdCACertificate            string   `json:"etcdCACertificate,omitempty"`
	EtcdCACertificateKey         string   `json:"etcdCACertificateKey,omitempty"`
	AdminKubeConfig              string   `json:"adminKubeConfig,omitempty"`
	CustomerKubeConfig           string   `json:"customerKubeConfig,omitempty"`
	DiscoveryHashes              []string `json:"discoveryHashes,omitempty"`
	PublicDNSName                string   `json:"publicDNSName,omitempty"`
	InternalDNSName              string   `json:"internalDNSName,omitempty"`
	AzureCloudProviderConfig     string   `json:"azureCloudProviderConfig,omitempty"`
	BootstrapVMSKUType           string   `json:"bootstrapVMSKUType,omitempty"`
	BootstrapKubernetesVersion   string   `json:"bootstrapKubernetesVersion,omitempty"`
}

func (in *Spec) DeepCopyInto(out *Spec) {
	*out = *in
	return
}

func CreateSpec(cloudConfig *azhelpers.CloudConfiguration, dnsPrefix, vmSKUType, kubernetesVersion string) (*Spec, error) {
	spec := &Spec{}
	if spec.ClusterName == "" {
		h := fnv.New64a()
		h.Write([]byte(fmt.Sprintf("%s/%s", cloudConfig.SubscriptionID, cloudConfig.GroupName)))
		spec.ClusterName = fmt.Sprintf("%x", h.Sum64())
	}
	if !spec.CloudConfiguration.IsValid() {
		spec.CloudConfiguration = *cloudConfig
	}
	if spec.DNSPrefix == "" {
		spec.DNSPrefix = dnsPrefix
	}
	if spec.BootstrapKubernetesVersion == "" {
		spec.BootstrapKubernetesVersion = kubernetesVersion
	}
	if spec.BootstrapVMSKUType == "" {
		spec.BootstrapVMSKUType = vmSKUType
	}

	publicIPName := ""

	{
		h := fnv.New32a()
		h.Write([]byte(fmt.Sprintf("%s-%s", azkPublicIPName, spec.ClusterName)))
		publicIPName = fmt.Sprintf("%x", h.Sum32())
	}

	publicIPName = dnsPrefix + publicIPName
	publicDNSName := fmt.Sprintf("%s.%s.cloudapp.azure.com", strings.ToLower(publicIPName), strings.ToLower(cloudConfig.GroupLocation))
	internalDNSName := fmt.Sprintf("%s.internal", strings.ToLower(publicIPName))

	if spec.PublicDNSName == "" {
		spec.PublicDNSName = publicDNSName
	} else {
		publicDNSName = spec.PublicDNSName
	}
	if spec.InternalDNSName == "" {
		spec.InternalDNSName = internalDNSName
	} else {
		internalDNSName = spec.InternalDNSName
	}

	tmpDirName := tmpDir + spec.ClusterName

	defer os.RemoveAll(tmpDirName)

	v1beta1cfg := &kubeadmv1beta1.InitConfiguration{}
	kubeadmscheme.Scheme.Default(v1beta1cfg)
	v1beta1cfg.CertificatesDir = tmpDirName + "/certs"
	v1beta1cfg.Etcd.Local = &kubeadmv1beta1.LocalEtcd{}
	v1beta1cfg.LocalAPIEndpoint = kubeadmv1beta1.APIEndpoint{AdvertiseAddress: "10.0.0.4", BindPort: 6443}
	v1beta1cfg.ControlPlaneEndpoint = fmt.Sprintf("%s:6443", internalDNSName)
	v1beta1cfg.APIServer.CertSANs = []string{"10.0.0.100", publicDNSName, internalDNSName}
	v1beta1cfg.NodeRegistration.Name = "fakenode" + spec.ClusterName
	cfg := &kubeadmapi.InitConfiguration{}
	kubeadmscheme.Scheme.Default(cfg)
	kubeadmscheme.Scheme.Convert(v1beta1cfg, cfg, nil)

	log.Info("Creating PKI Certificates", "InternalDNS", internalDNSName)
	if err := CreatePKISACertificates(cfg); err != nil {
		log.Error(err, "Error Generating Certificates")
		return nil, err
	}
	log.Info("Successfully Created PKI Certificates", "InternalDNS", internalDNSName)

	log.Info("Creating Kubeconfigs")
	kubeConfigDir := tmpDirName + "/kubeconfigs"
	if err := CreateKubeconfigs(cfg, kubeConfigDir); err != nil {
		log.Error(err, "Error Generating Kubeconfigs")
		return nil, err
	}
	log.Info("Successfully Created Kubeconfigs")

	if err := spec.UpdateSpec(); err != nil {
		log.Error(err, "Error Updating Status")
		return nil, err
	}

	if spec.AzureCloudProviderConfig == "" {
		azureCloudProviderConfig := azhelpers.GetAzureCloudProviderConfig(cloudConfig)
		spec.AzureCloudProviderConfig = azureCloudProviderConfig
	}

	if spec.CustomerKubeConfig == "" {
		log.Info("Creating Customer Kubeconfig", "DNS", publicDNSName)
		os.Remove(tmpDirName + "/kubeconfigs/admin.conf")
		cfg.LocalAPIEndpoint = kubeadmapi.APIEndpoint{AdvertiseAddress: "10.0.0.4", BindPort: 6443}
		cfg.ControlPlaneEndpoint = fmt.Sprintf("%s:443", publicDNSName)
		if err := kubeconfigphase.CreateKubeConfigFile(kubeadmconstants.AdminKubeConfigFileName, kubeConfigDir, cfg); err != nil {
			return nil, err
		}
		buf, err := ioutil.ReadFile(tmpDirName + "/kubeconfigs/admin.conf")
		if err != nil {
			return nil, err
		}
		spec.CustomerKubeConfig = string(buf)
		log.Info("Created Customer Kubeconfig", "DNS", publicDNSName)
	}

	return spec, nil
}

func (spec *Spec) UpdateSpec() error {
	tmpDirName := tmpDir + spec.ClusterName
	if spec.CACertificate == "" {
		buf, err := ioutil.ReadFile(tmpDirName + "/certs/ca.crt")
		if err != nil {
			return err
		}
		spec.CACertificate = string(buf)
	}
	if spec.CACertificateKey == "" {
		buf, err := ioutil.ReadFile(tmpDirName + "/certs/ca.key")
		if err != nil {
			return err
		}
		spec.CACertificateKey = string(buf)
	}
	if spec.ServiceAccountKey == "" {
		buf, err := ioutil.ReadFile(tmpDirName + "/certs/sa.key")
		if err != nil {
			return err
		}
		spec.ServiceAccountKey = string(buf)
	}
	if spec.ServiceAccountPub == "" {
		buf, err := ioutil.ReadFile(tmpDirName + "/certs/sa.pub")
		if err != nil {
			return err
		}
		spec.ServiceAccountPub = string(buf)
	}
	if spec.FrontProxyCACertificate == "" {
		buf, err := ioutil.ReadFile(tmpDirName + "/certs/front-proxy-ca.crt")
		if err != nil {
			return err
		}
		spec.FrontProxyCACertificate = string(buf)
	}

	if spec.FrontProxyCACertificateKey == "" {
		buf, err := ioutil.ReadFile(tmpDirName + "/certs/front-proxy-ca.key")
		if err != nil {
			return err
		}
		spec.FrontProxyCACertificateKey = string(buf)
	}

	if spec.EtcdCACertificate == "" {
		buf, err := ioutil.ReadFile(tmpDirName + "/certs/etcd/ca.crt")
		if err != nil {
			return err
		}
		spec.EtcdCACertificate = string(buf)
	}

	if spec.EtcdCACertificateKey == "" {
		buf, err := ioutil.ReadFile(tmpDirName + "/certs/etcd/ca.key")
		if err != nil {
			return err
		}
		spec.EtcdCACertificateKey = string(buf)
	}
	if spec.AdminKubeConfig == "" {
		buf, err := ioutil.ReadFile(tmpDirName + "/kubeconfigs/admin.conf")
		if err != nil {
			return err
		}
		spec.AdminKubeConfig = string(buf)
	}

	// Discovery hashes typically never changes
	if len(spec.DiscoveryHashes) <= 0 {
		discoveryHashes, err := GetDiscoveryHashes(tmpDirName + "/kubeconfigs/admin.conf")
		if err != nil {
			return err
		}
		spec.DiscoveryHashes = discoveryHashes
	}
	return nil
}

func CreateNewBootstrapToken() (string, error) {
	token, err := bootstraputil.GenerateBootstrapToken()
	if err != nil {
		return token, err
	}

	cfg, err := config.GetConfig()
	if err != nil {
		return token, err
	}

	kclientset, err := clientset.NewForConfig(cfg)
	if err != nil {
		return token, err
	}

	tokenString, err := kubeadmapi.NewBootstrapTokenString(token)
	if err != nil {
		return token, err
	}

	bootstrapTokens := []kubeadmapi.BootstrapToken{
		kubeadmapi.BootstrapToken{
			Token:  tokenString,
			TTL:    &metav1.Duration{Duration: 1 * time.Hour},
			Groups: []string{"system:bootstrappers:kubeadm:default-node-token"},
			Usages: []string{"signing", "authentication"},
		},
	}

	if err := tokenphase.CreateNewTokens(kclientset, bootstrapTokens); err != nil {
		return token, err
	}

	return token, nil
}
