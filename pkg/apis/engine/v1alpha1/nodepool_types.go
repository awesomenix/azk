package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodePoolSpec defines the desired state of NodePool
type NodePoolSpec struct {
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`
	Replicas          *int32 `json:"replicas,omitempty"`
}

// NodePoolStatus defines the observed state of NodePool
type NodePoolStatus struct {
	NodeSetName string `json:"nodesetName,omitempty"`
	//PrevNodeSetName   string `json:"prevNodeSetName,omitempty"`
	Replicas          int32  `json:"replicas,omitempty"`
	VMReplicas        int32  `json:"vmreplicas,omitempty"`
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`
	ProvisioningState string `json:"provisioningState,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodePool is the Schema for the nodepools API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
type NodePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodePoolSpec   `json:"spec,omitempty"`
	Status NodePoolStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodePoolList contains a list of NodePool
type NodePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodePool{}, &NodePoolList{})
}
