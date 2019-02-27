package nodeset

import (
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	kubectldrain "k8s.io/kubernetes/pkg/kubectl/cmd/drain"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

type restClientGetter struct {
	config clientcmd.ClientConfig
}

func (r *restClientGetter) ToRESTConfig() (*rest.Config, error) {
	clientConfig, err := r.ToRawKubeConfigLoader().ClientConfig()
	if err != nil {
		return nil, err
	}
	clientConfig.Timeout = 5 * time.Second
	return clientConfig, err
}

func (r *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := r.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	return discovery.NewCachedDiscoveryClientForConfig(config, os.TempDir(), "", 10*time.Minute)
}

func (r *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	client, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(client), nil
}

func (r *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return r.config
}

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
	f := cmdutil.NewFactory(&restClientGetter{config: clientConfig})

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
