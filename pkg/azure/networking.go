package azhelpers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-01-01/network"
	"github.com/Azure/go-autorest/autorest/to"
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

func (c *CloudConfiguration) GetVNETPeeringsClient() (network.VirtualNetworkPeeringsClient, error) {
	peeringsClient := network.NewVirtualNetworkPeeringsClient(c.SubscriptionID)
	auth, err := c.getAuthorizerForResource()
	if err != nil {
		return peeringsClient, err
	}
	peeringsClient.Authorizer = auth
	peeringsClient.AddToUserAgent(c.UserAgent)
	return peeringsClient, nil
}

func (c *CloudConfiguration) GetVNETClient() (network.VirtualNetworksClient, error) {
	vnetClient := network.NewVirtualNetworksClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return vnetClient, err
	}
	vnetClient.Authorizer = a
	vnetClient.AddToUserAgent(c.UserAgent)
	return vnetClient, nil
}

func (c *CloudConfiguration) CreateVirtualNetworkAndSubnets(ctx context.Context, vnetName, groupName, groupLocation string) error {
	vnetClient, err := c.GetVNETClient()
	if err != nil {
		return err
	}
	future, err := vnetClient.CreateOrUpdate(
		ctx,
		groupName,
		vnetName,
		network.VirtualNetwork{
			Location: to.StringPtr(groupLocation),
			VirtualNetworkPropertiesFormat: &network.VirtualNetworkPropertiesFormat{
				AddressSpace: &network.AddressSpace{
					AddressPrefixes: &[]string{"192.0.0.0/8"},
				},
				Subnets: &[]network.Subnet{
					{
						Name: to.StringPtr("master-subnet"),
						SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
							AddressPrefix: to.StringPtr("192.0.0.0/16"),
						},
					},
					{
						Name: to.StringPtr("agent-subnet"),
						SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
							AddressPrefix: to.StringPtr("192.1.0.0/16"),
						},
					},
				},
			},
		})

	if err != nil {
		return fmt.Errorf("cannot create virtual network: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, vnetClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get the vnet create or update future response: %v", err)
	}

	_, err = future.Result(vnetClient)

	return err
}
