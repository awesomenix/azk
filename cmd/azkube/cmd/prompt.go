package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var flowCmd = &cobra.Command{
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

func init() {
	createClusterCmd.AddCommand(flowCmd)
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

	copt := &CreateOptions{
		SubscriptionID:    subscriptionID,
		ClientID:          clientID,
		ClientSecret:      clientSecret,
		TenantID:          "72f988bf-86f1-41af-91ab-2d7cd011db47",
		ResourceGroup:     resourceGroupName,
		ResourceLocation:  region,
		DNSPrefix:         dnsPrefix,
		KubernetesVersion: "stable",
		NodePoolName:      "nodepool1",
		NodePoolCount:     int32(nodeCount),
		KubeconfigOutput:  "kubeconfig",
	}

	return RunCreate(copt)
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
	prompt := promptui.Select{
		Label: "Select Region",
		Items: locations,
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
	subClient.AddToUserAgent("azkubeprompt")

	res, err := subClient.ListLocations(context.TODO(), subscriptionID)
	if err != nil {
		return locations, err
	}

	for _, location := range *res.Value {
		locations = append(locations, *location.Name)
	}
	return locations, err
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
