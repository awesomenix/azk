package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeSetSpec defines the desired state of NodeSet
type NodeSetSpec struct {
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`
	Replicas          *int32 `json:"replicas,omitempty"`
}

// NodeSetStatus defines the observed state of NodeSet
type NodeSetStatus struct {
	Replicas          int32             `json:"replicas,omitempty"`
	KubernetesVersion string            `json:"kubernetesVersion,omitempty"`
	ProvisioningState string            `json:"provisioningState,omitempty"`
	Kubeconfig        string            `json:"kubeConfig,omitempty"`
	NodeStatus        []NodeSetVMStatus `json:"nodeStatus,omitempty"`
}

type NodeSetVMStatus struct {
	VMComputerName string `json:"vmComputerName,omitempty"`
	VMInstanceID   string `json:"vmInstanceID,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeSet is the Schema for the nodesets API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
type NodeSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeSetSpec   `json:"spec,omitempty"`
	Status NodeSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeSetList contains a list of NodeSet
type NodeSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeSet{}, &NodeSetList{})
}
