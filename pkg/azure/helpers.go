package azhelpers

import (
	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-03-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2019-03-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
)

const (
	AzurePublicCloudName = "AzurePublicCloud"
)

type CloudConfiguration struct {
	CloudName      string
	SubscriptionID string
	ClientID       string
	ClientSecret   string
	TenantID       string
	GroupName      string
	GroupLocation  string
	UserAgent      string
}

func (c *CloudConfiguration) getAuthorizerForResource() (autorest.Authorizer, error) {
	env, err := azure.EnvironmentFromName(c.CloudName)
	if err != nil {
		return nil, err
	}
	oauthConfig, err := adal.NewOAuthConfig(
		env.ActiveDirectoryEndpoint, c.TenantID)
	if err != nil {
		return nil, err
	}

	token, err := adal.NewServicePrincipalToken(
		*oauthConfig, c.ClientID, c.ClientSecret, env.ResourceManagerEndpoint)
	if err != nil {
		return nil, err
	}
	return autorest.NewBearerAuthorizer(token), nil
}

func (c *CloudConfiguration) IsValid() bool {
	if c.CloudName != "" &&
		c.SubscriptionID != "" &&
		c.ClientID != "" &&
		c.ClientSecret != "" &&
		c.TenantID != "" {
		return true
	}
	return false
}

func (c *CloudConfiguration) GetResourcesClient() (resources.Client, error) {
	resourcesClient := resources.NewClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return resourcesClient, err
	}
	resourcesClient.Authorizer = a
	resourcesClient.AddToUserAgent(c.UserAgent)
	return resourcesClient, nil
}

func (c *CloudConfiguration) GetDisksClient() (compute.DisksClient, error) {
	disksClient := compute.NewDisksClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return disksClient, err
	}
	disksClient.Authorizer = a
	disksClient.AddToUserAgent(c.UserAgent)
	return disksClient, nil
}

func (c *CloudConfiguration) GetDeploymentsClient() (resources.DeploymentsClient, error) {
	deploymentsClient := resources.NewDeploymentsClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return deploymentsClient, err
	}
	deploymentsClient.Authorizer = a
	deploymentsClient.AddToUserAgent(c.UserAgent)
	return deploymentsClient, nil
}
