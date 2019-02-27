package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	SubscriptionID    string `json:"subscriptionID,omitempty"`
	ResourceGroupName string `json:"resourceGroupName,omitempty"`
	ResourceName      string `json:"resourceName,omitempty"`
	DNSPrefix         string `json:"dnsPrefix,omitempty"`
	Location          string `json:"location,omitempty"`
	TenantID          string `json:"tenantID,omitempty"`
	ClientID          string `json:"clientID,omitempty"`
	ClientSecret      string `json:"clientSecret,omitempty"`
}

// ClusterStatus defines the observed state of Cluster
type ClusterStatus struct {
	ProvisioningState          string   `json:"provisioningState,omitempty"`
	CACertificate              string   `json:"caCertificate,omitempty"`
	CACertificateKey           string   `json:"caCertificateKey,omitempty"`
	ServiceAccountKey          string   `json:"serviceAccountKey,omitempty"`
	ServiceAccountPub          string   `json:"serviceAccountPub,omitempty"`
	FrontProxyCACertificate    string   `json:"frontProxyCACertificate,omitempty"`
	FrontProxyCACertificateKey string   `json:"frontProxyCACertificateKey,omitempty"`
	EtcdCACertificate          string   `json:"etcdCACertificate,omitempty"`
	EtcdCACertificateKey       string   `json:"etcdCACertificateKey,omitempty"`
	AdminKubeConfig            string   `json:"adminKubeConfig,omitempty"`
	CustomerKubeConfig         string   `json:"customerKubeConfig,omitempty"`
	BootstrapToken             string   `json:"bootstrapToken,omitempty"`
	DiscoveryHashes            []string `json:"discoveryHashes,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Cluster is the Schema for the clusters API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}
