package nodeset

import (
	"fmt"
	"os"

	"github.com/awesomenix/azkube/pkg/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
	kubectldrain "k8s.io/kubernetes/pkg/kubectl/cmd/drain"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

func cordonDrainAndDeleteNode(kubeconfig string, vmName string) error {
	if kubeconfig == "" {
		// empty kubeconfig, skip cordon and delete
		return nil
	}
	log.Info("Cordon and Drain", "VMName", vmName)
	clientConfig, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfig))
	if err != nil {
		return fmt.Errorf("error setting up kubeconfig: %v", err)
	}
	f := cmdutil.NewFactory(&helpers.RestClientGetter{Config: clientConfig})

	streams := genericclioptions.IOStreams{
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	drain := kubectldrain.NewCmdDrain(f, streams)
	args := []string{vmName}
	options := kubectldrain.NewDrainOptions(f, streams)

	// Override some options
	options.IgnoreDaemonsets = true
	options.Force = true
	options.DeleteLocalData = true
	options.GracePeriodSeconds = 60

	err = options.Complete(f, drain, args)
	if err != nil {
		return fmt.Errorf("error setting up drain: %v", err)
	}

	log.Info("Cordon", "VMName", vmName)
	err = options.RunCordonOrUncordon(true)
	if err != nil {
		return fmt.Errorf("error cordoning node: %v", err)
	}

	log.Info("Draining", "VMName", vmName)

	err = options.RunDrain()
	if err != nil {
		return fmt.Errorf("error draining node: %v", err)
	}

	clientSet, err := f.KubernetesClientSet()
	if err != nil {
		return fmt.Errorf("failed to get clientset: %v", err)
	}

	log.Info("Deleting", "VMName", vmName)
	return clientSet.CoreV1().Nodes().Delete(vmName, &metav1.DeleteOptions{})
}
