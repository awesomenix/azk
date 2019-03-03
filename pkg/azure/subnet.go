package azhelpers

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-01-01/network"
)

func (c *CloudConfiguration) GetSubnetsClient() (network.SubnetsClient, error) {
	subnetsClient := network.NewSubnetsClient(c.SubscriptionID)
	auth, err := c.getAuthorizerForResource()
	if err != nil {
		return subnetsClient, err
	}
	subnetsClient.Authorizer = auth
	subnetsClient.AddToUserAgent(c.UserAgent)
	return subnetsClient, nil
}

// GetSubnet returns an existing subnet from a virtual network
func (c *CloudConfiguration) GetSubnet(ctx context.Context, vnetName, subnetName string) (network.Subnet, error) {
	subnetsClient, err := c.GetSubnetsClient()
	if err != nil {
		return network.Subnet{}, err
	}
	return subnetsClient.Get(ctx, c.GroupName, vnetName, subnetName, "")
}
