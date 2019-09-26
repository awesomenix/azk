package helpers

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/awesomenix/azk/helpers"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
	kubectlapply "k8s.io/kubernetes/pkg/kubectl/cmd/apply"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("azk")

func KubectlApply(manifestPath, kubeconfig string) error {
	clientcfg, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfig))
	if err != nil {
		return err
	}

	f := cmdutil.NewFactory(&helpers.RestClientGetter{Config: clientcfg})

	streams := genericclioptions.IOStreams{
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	apply := kubectlapply.NewCmdApply("kubectl", f, streams)
	args := []string{manifestPath}
	options := kubectlapply.NewApplyOptions(streams)

	err = options.Complete(f, apply)
	if err != nil {
		return fmt.Errorf("error setting up apply: %v", err)
	}

	options.DeleteOptions.FilenameOptions.Filenames = args
	options.DeleteOptions.FilenameOptions.Recursive = true

	err = options.Run()
	if err != nil {
		return fmt.Errorf("failed to apply %s: %v", manifestPath, err)
	}

	return nil
}

func KubectlApplyFolder(folder string, kubeconfig string, fs http.FileSystem) error {
	const azkAssetDir = "/tmp/azk-assets/"
	tmpAssetsDir := azkAssetDir + folder
	defer os.RemoveAll(tmpAssetsDir)

	f, err := fs.Open(folder)
	if err != nil {
		log.Error(err, "Failed to open assets folder", "Folder", folder)
		return err
	}
	defer f.Close()
	fi, err := f.Readdir(-1)
	if err != nil {
		log.Error(err, "Failed to read assets folder", "Folder", folder)
		return err
	}
	for _, fi := range fi {
		if !fi.IsDir() {
			assetFileName := folder + "/" + fi.Name()
			file, err := fs.Open(assetFileName)
			if err != nil {
				log.Error(err, "Failed to open file from assets", "File", assetFileName)
				return err
			}
			defer file.Close()
			bytes, err := ioutil.ReadAll(file)
			if err != nil {
				log.Error(err, "Failed to read asset file", "File", assetFileName)
				return err
			}
			os.MkdirAll(tmpAssetsDir, os.ModePerm)
			fileName := azkAssetDir + "/" + assetFileName
			err = ioutil.WriteFile(fileName, bytes, 0644)
			if err != nil {
				log.Error(err, "Failed to write file", "File", fileName)
				return err
			}
		}
	}

	if err := KubectlApply(tmpAssetsDir, kubeconfig); err != nil {
		log.Error(err, "Failed to apply to cluster", "Resource", folder)
	}
	return nil
}
