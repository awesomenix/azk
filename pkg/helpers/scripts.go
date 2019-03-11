package helpers

import "fmt"

func PreRequisitesInstallScript(kubernetesVersion string) string {
	return fmt.Sprintf(`
sudo apt-get update && sudo apt-get install -y apt-transport-https ca-certificates curl gnupg-agent software-properties-common
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
sudo apt-get install -y docker-ce=18.06.0~ce~3-0~ubuntu containerd.io
curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
cat <<EOF >/tmp/kubernetes.list
deb https://apt.kubernetes.io/ kubernetes-xenial main
EOF
sudo mv /tmp/kubernetes.list /etc/apt/sources.list.d/kubernetes.list
sudo apt-get update && sudo apt-get install -y kubelet=%[1]s-00 kubectl=%[1]s-00 kubeadm
sudo apt-mark hold kubelet kubeadm kubectl
sudo sysctl net.bridge.bridge-nf-call-iptables=1
`, kubernetesVersion)
}

func FlannelCNI() string {
	return fmt.Sprintf(`
#flannel use 10.244.0.0/16 as podsubnet
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel.yml
`)
}

func CanalCNI() string {
	return fmt.Sprintf(`
#cancal use 10.244.0.0/16 as podsubnet
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f https://docs.projectcalico.org/v3.5/getting-started/kubernetes/installation/hosted/canal/canal.yaml
`)
}

func CalicoCNI() string {
	return fmt.Sprintf(`
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f https://docs.projectcalico.org/v3.5/getting-started/kubernetes/installation/hosted/kubernetes-datastore/calico-networking/1.7/calico.yaml
`)
}

func RomanaCNI() string {
	return fmt.Sprintf(`
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f https://raw.githubusercontent.com/romana/romana/master/docs/kubernetes/romana-kubeadm.yml
`)
}

func KuberouterCNI() string {
	return fmt.Sprintf(`
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f https://raw.githubusercontent.com/cloudnativelabs/kube-router/master/daemonset/kubeadm-kuberouter.yaml
#below uses kuberouter for service communication as well
#testing required since kubeadm has KubproxyConfig
#sudo kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f https://raw.githubusercontent.com/cloudnativelabs/kube-router/master/daemonset/kubeadm-kuberouter-all-features.yaml
#sudo kubectl --kubeconfig /etc/kubernetes/admin.conf -n kube-system delete ds kube-proxy
#sudo docker run --privileged -v /lib/modules:/lib/modules --net=host k8s.gcr.io/kube-proxy-amd64:v1.10.2 kube-proxy --cleanup
`)
}
