package helpers

import (
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
	kubectldrain "k8s.io/kubernetes/pkg/kubectl/cmd/drain"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

func CordonDrainAndDeleteNode(kubeconfig string, vmName string) error {
	if kubeconfig == "" {
		// empty kubeconfig, skip cordon and delete
		return nil
	}
	log.Info("Cordon and Drain", "VMName", vmName)
	clientConfig, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfig))
	if err != nil {
		return fmt.Errorf("error setting up kubeconfig: %v", err)
	}
	f := cmdutil.NewFactory(&RestClientGetter{Config: clientConfig})

	streams := genericclioptions.IOStreams{
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
	drain := kubectldrain.NewCmdDrain(f, streams)

	drain.SetArgs([]string{
		vmName,
		"--force",
		"--ignore-daemonsets",
		"--grace-period",
		"60",
		"--delete-local-data",
	})

	var drainerr error
	cmdutil.BehaviorOnFatal(func(msg string, code int) {
		drainerr = fmt.Errorf("error during drain: %s", msg)
	})
	err = drain.Execute()
	if err != nil {
		return err
	}
	if drainerr != nil {
		return drainerr
	}

	clientSet, err := f.KubernetesClientSet()
	if err != nil {
		return fmt.Errorf("failed to get clientset: %v", err)
	}

	log.Info("Deleting", "VMName", vmName)
	return clientSet.CoreV1().Nodes().Delete(vmName, &metav1.DeleteOptions{})
}
