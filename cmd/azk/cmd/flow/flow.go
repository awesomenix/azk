package flow

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/awesomenix/azk/cmd/azk/cmd/cluster"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("azk")

var FlowCmd = &cobra.Command{
	Use:   "flow",
	Short: "Create a Kubernetes Cluster on Azure",
	Long:  `Create a Kubernetes Cluster on Azure with interactive flow`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RunFlow(); err != nil {
			log.Error(err, "Failed to create flow")
			os.Exit(1)
		}
	},
}

func RunFlow() error {
	subscriptionID, err := getSecret("Subscription ID", func(i string) error {
		if len(i) < 30 {
			return fmt.Errorf("Invalid Subscription ID")
		}
		return nil
	})

	if err != nil {
		return err
	}

	clientID, err := getSecret("Service Principal ClientID", func(i string) error {
		return nil
	})
	if err != nil {
		return err
	}

	clientSecret, err := getSecret("Service Principal ClientSecret", func(i string) error {
		return nil
	})
	if err != nil {
		return err
	}

	tenantID, err := getSecret("Tenant ID", func(i string) error {
		return nil
	})
	if err != nil {
		return err
	}

	resourceGroupName, err := getInput("Resource Group Name", func(i string) error {
		matched, err := regexp.MatchString(`^[-\w\._\(\)]+$`, i)
		if err != nil || !matched {
			return fmt.Errorf("Invalid Resource Group Name")
		}
		return nil
	})

	if err != nil {
		return err
	}

	dnsPrefix, err := getInput("DNS Prefix", func(i string) error {
		matched, err := regexp.MatchString(`^[a-zA-Z][-a-zA-Z0-9]{0,43}[a-zA-Z0-9]$`, i)
		if err != nil || !matched {
			return fmt.Errorf("Invalid DNS Prefix")
		}
		return nil
	})
	if err != nil {
		return err
	}

	region, err := selectRegion(subscriptionID, clientID, clientSecret)
	if err != nil {
		return err
	}

	vmsize, err := selectVMSize(subscriptionID, region, clientID, clientSecret)
	if err != nil {
		return err
	}

	nodepoolCount, err := getInput("Node Pool Count", func(i string) error {
		_, err := strconv.ParseUint(i, 10, 32)
		if err != nil {
			return fmt.Errorf("Invalid number")
		}
		return nil
	})
	if err != nil {
		return err
	}

	nodeCount, _ := strconv.ParseUint(nodepoolCount, 10, 32)

	copt := &cluster.CreateOptions{
		SubscriptionID:    subscriptionID,
		ClientID:          clientID,
		ClientSecret:      clientSecret,
		TenantID:          tenantID,
		ResourceGroup:     resourceGroupName,
		ResourceLocation:  region,
		DNSPrefix:         dnsPrefix,
		KubernetesVersion: "stable",
		NodePoolName:      "nodepool1",
		NodePoolCount:     int32(nodeCount),
		VMSKUType:         vmsize,
		KubeconfigOutput:  "kubeconfig",
	}

	return cluster.RunCreate(copt)
}

func getInput(label string, validate func(string) error) (string, error) {
	prompt := promptui.Prompt{
		Label:    label,
		Validate: validate,
	}

	result, err := prompt.Run()

	if err == promptui.ErrInterrupt {
		os.Exit(-1)
	}

	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return "", err
	}

	return result, nil
}

func getSecret(label string, validate func(string) error) (string, error) {
	prompt := promptui.Prompt{
		Label:    label,
		Validate: validate,
		Mask:     '*',
	}

	result, err := prompt.Run()

	if err == promptui.ErrInterrupt {
		os.Exit(-1)
	}

	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return "", err
	}

	return result, nil
}

func selectRegion(subscriptionID, clientID, clientSecret string) (string, error) {
	locations, err := getLocations(subscriptionID, clientID, clientSecret)
	if err != nil {
		return "", err
	}
	sort.Strings(locations)
	searcher := func(input string, index int) bool {
		location := locations[index]
		name := strings.Replace(strings.ToLower(location), " ", "", -1)
		input = strings.Replace(strings.ToLower(input), " ", "", -1)

		return strings.Contains(name, input)
	}
	prompt := promptui.Select{
		Label:    "Select Region",
		Items:    locations,
		Searcher: searcher,
	}

	_, result, err := prompt.Run()

	if err == promptui.ErrInterrupt {
		os.Exit(-1)
	}

	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return "", err
	}
	return result, nil
}

func selectVMSize(subscriptionID, location, clientID, clientSecret string) (string, error) {
	vmSizes, err := getVMSizes(subscriptionID, location, clientID, clientSecret)
	if err != nil {
		return "", err
	}
	sort.Strings(vmSizes)
	searcher := func(input string, index int) bool {
		vmSize := vmSizes[index]
		name := strings.Replace(strings.ToLower(vmSize), " ", "", -1)
		input = strings.Replace(strings.ToLower(input), " ", "", -1)

		return strings.Contains(name, input)
	}

	prompt := promptui.Select{
		Label:    "Select VM Size",
		Items:    vmSizes,
		Searcher: searcher,
	}

	_, result, err := prompt.Run()

	if err == promptui.ErrInterrupt {
		os.Exit(-1)
	}

	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return "", err
	}
	return result, nil
}

func getLocations(subscriptionID, clientID, clientSecret string) ([]string, error) {
	var locations []string
	subClient := subscription.NewSubscriptionsClient()
	a, err := getAuthorizerForResource(subscriptionID, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	subClient.Authorizer = a
	subClient.AddToUserAgent("azkprompt")

	res, err := subClient.ListLocations(context.TODO(), subscriptionID)
	if err != nil {
		return locations, err
	}

	for _, location := range *res.Value {
		locations = append(locations, *location.Name)
	}
	return locations, err
}

// func getResourcesSku(subscriptionID, clientID, clientSecret string) ([]string, error) {
// 	var locations []string
// 	resClient := compute.NewResourceSkusClient(subscriptionID)
// 	a, err := getAuthorizerForResource(subscriptionID, clientID, clientSecret)
// 	if err != nil {
// 		return nil, err
// 	}
// 	resClient.Authorizer = a
// 	resClient.AddToUserAgent("azkprompt")

// 	res, err := resClient.List(context.TODO())
// 	if err != nil {
// 		return locations, err
// 	}

// 	for _, resSku := range res.Values() {
// 		fmt.Println(*resSku.Locations)
// 		fmt.Println(*resSku.Name)
// 		for _, locationInfo := range *resSku.LocationInfo {
// 			fmt.Println(*locationInfo.Location)
// 			fmt.Println(*locationInfo.Zones)
// 		}

// 	}
// 	return nil, nil
// }

func getVMSizes(subscriptionID, location, clientID, clientSecret string) ([]string, error) {
	var vmsizes []string
	vmSizesClient := compute.NewVirtualMachineSizesClient(subscriptionID)
	a, err := getAuthorizerForResource(subscriptionID, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	vmSizesClient.Authorizer = a
	vmSizesClient.AddToUserAgent("azkprompt")

	res, err := vmSizesClient.List(context.TODO(), location)
	if err != nil {
		return vmsizes, err
	}

	for _, vmsize := range *res.Value {
		vmsizes = append(vmsizes, *vmsize.Name)
	}
	return vmsizes, err
}

func getAuthorizerForResource(subscriptionID, clientID, clientSecret string) (autorest.Authorizer, error) {
	env, err := azure.EnvironmentFromName("AzurePublicCloud")
	if err != nil {
		return nil, err
	}
	oauthConfig, err := adal.NewOAuthConfig(
		env.ActiveDirectoryEndpoint, "72f988bf-86f1-41af-91ab-2d7cd011db47")
	if err != nil {
		return nil, err
	}

	token, err := adal.NewServicePrincipalToken(
		*oauthConfig, clientID, clientSecret, env.ResourceManagerEndpoint)
	if err != nil {
		return nil, err
	}
	return autorest.NewBearerAuthorizer(token), nil
}
