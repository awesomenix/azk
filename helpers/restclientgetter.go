package helpers

import (
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/disk"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

type RestClientGetter struct {
	Config clientcmd.ClientConfig
}

func (r *RestClientGetter) ToRESTConfig() (*rest.Config, error) {
	clientConfig, err := r.ToRawKubeConfigLoader().ClientConfig()
	if err != nil {
		return nil, err
	}
	clientConfig.Timeout = 5 * time.Second
	return clientConfig, err
}

func (r *RestClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := r.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	return disk.NewCachedDiscoveryClientForConfig(config, os.TempDir(), "", 10*time.Minute)
}

func (r *RestClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	client, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(client), nil
}

func (r *RestClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return r.Config
}
