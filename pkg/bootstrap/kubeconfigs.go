package bootstrap

import (
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	kubeconfigphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"
)

func CreateKubeconfigs(cfg *kubeadmapi.InitConfiguration, kubeConfigDir string) error {
	if err := kubeconfigphase.CreateKubeConfigFile(kubeadmconstants.AdminKubeConfigFileName, kubeConfigDir, cfg); err != nil {
		return err
	}
	if err := kubeconfigphase.CreateKubeConfigFile(kubeadmconstants.KubeletKubeConfigFileName, kubeConfigDir, cfg); err != nil {
		return err
	}
	if err := kubeconfigphase.CreateKubeConfigFile(kubeadmconstants.ControllerManagerKubeConfigFileName, kubeConfigDir, cfg); err != nil {
		return err
	}
	if err := kubeconfigphase.CreateKubeConfigFile(kubeadmconstants.SchedulerKubeConfigFileName, kubeConfigDir, cfg); err != nil {
		return err
	}
	return nil
}
