package helpers

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	debugruntime "runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("controller")

var letterRunes = []rune("0123456789abcdef")

// generateRandomHexString is a convenience function for generating random strings of an arbitrary length.
func GenerateRandomHexString(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func ContainsFinalizer(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func RemoveFinalizer(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func GetKubernetesVersion(version string) (string, error) {
	if version == "stable" || version == "latest" {
		resp, err := http.Get("https://storage.googleapis.com/kubernetes-release/release/" + version + ".txt")
		if err != nil {
			return version, err
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return version, err
		}

		return strings.TrimSpace(string(body[1:])), nil
	}
	return version, nil
}

func GetStableUpgradeKubernetesVersion(version string) (string, error) {
	if version == "stable" || version == "latest" {
		return GetKubernetesVersion(version)
	}
	ver, err := semver.NewVersion(version)
	if err != nil {
		return version, err
	}
	queryVersion := fmt.Sprintf("%d.%d", ver.Major(), ver.Minor())
	resp, err := http.Get("https://storage.googleapis.com/kubernetes-release/release/stable-" + queryVersion + ".txt")
	if err != nil {
		return version, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return version, err
	}

	return strings.TrimSpace(string(body[1:])), nil
}

func GetLatestUpgradeKubernetesVersion(version string) (string, error) {
	if version == "stable" || version == "latest" {
		return GetKubernetesVersion(version)
	}
	ver, err := semver.NewVersion(version)
	if err != nil {
		return version, err
	}
	queryVersion := fmt.Sprintf("%d.%d", ver.Major(), ver.Minor())
	resp, err := http.Get("https://storage.googleapis.com/kubernetes-release/release/latest-" + queryVersion + ".txt")
	if err != nil {
		return version, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return version, err
	}

	return strings.TrimSpace(string(body[1:])), nil
}

func WaitForNodesReady(kclient client.Client, nodeName string, nodeCount int) error {
	listOptions := &client.ListOptions{
		Namespace: "",
		Raw: &metav1.ListOptions{
			TypeMeta: metav1.TypeMeta{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Node",
			},
		},
	}

	var localErr error
	for i := 0; i < 100; i++ {
		nodeList := &corev1.NodeList{}
		if err := kclient.List(context.TODO(), listOptions, nodeList); err != nil {
			return err
		}

		foundNodes := 0
		for _, node := range nodeList.Items {
			if strings.Contains(node.Name, nodeName) {
				for _, c := range node.Status.Conditions {
					if c.Type == corev1.NodeReady {
						foundNodes++
						break
					}
				}
			}
		}

		if foundNodes < nodeCount {
			localErr = fmt.Errorf("Found %d nodes, expected %d nodes to be Ready", foundNodes, nodeCount)
			time.Sleep(3 * time.Second)
			continue
		}
		localErr = nil
		break
	}

	return localErr
}

func GetKubeClient(kubeconfig string) (client.Client, error) {
	clientcfg, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfig))
	if err != nil {
		log.Error(err, "Failed to NewClientConfigFromBytes")
		return nil, err
	}

	cfg, err := clientcfg.ClientConfig()
	if err != nil {
		log.Error(err, "Failed to get client config")
		return nil, err
	}
	kclient, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Error(err, "Failed to create kube client from config")
		return nil, err
	}
	return kclient, nil
}

func Recover() {
	// recover from panic if one occured. Set err to nil otherwise.
	if r := recover(); r != nil {
		_, file, line, _ := debugruntime.Caller(3)
		stack := string(debug.Stack())
		log.Error(fmt.Errorf("Panic: %+v, file: %s, line: %d, stacktrace: '%s'", r, file, line, stack), "Panic Observed")
	}
}
