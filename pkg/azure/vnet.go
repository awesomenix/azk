package azhelpers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-12-01/network"
	"github.com/Azure/go-autorest/autorest/to"
)

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

func (c *CloudConfiguration) CreateVirtualNetworkAndSubnets(ctx context.Context, vnetName string) error {
	vnetClient, err := c.GetVNETClient()
	if err != nil {
		return err
	}

	networkSecurityGroup, err := c.CreateDefaultNetworkSecurityGroup(context.TODO(), "azk-nsg")
	if err != nil {
		return err
	}

	masterNetworkSecurityGroup, err := c.CreateNetworkSecurityGroup(context.TODO(), "azk-master-nsg")
	if err != nil {
		return err
	}

	routeTable, err := c.CreateRouteTables(context.TODO(), "azk-routetable")
	if err != nil {
		return err
	}

	future, err := vnetClient.CreateOrUpdate(
		ctx,
		c.GroupName,
		vnetName,
		network.VirtualNetwork{
			Location: to.StringPtr(c.GroupLocation),
			VirtualNetworkPropertiesFormat: &network.VirtualNetworkPropertiesFormat{
				AddressSpace: &network.AddressSpace{
					AddressPrefixes: &[]string{"10.0.0.0/8"},
				},
				Subnets: &[]network.Subnet{
					{
						Name: to.StringPtr("master-subnet"),
						SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
							AddressPrefix:        to.StringPtr("10.0.0.0/16"),
							NetworkSecurityGroup: &masterNetworkSecurityGroup,
						},
					},
					{
						Name: to.StringPtr("agent-subnet"),
						SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
							AddressPrefix:        to.StringPtr("10.1.0.0/16"),
							NetworkSecurityGroup: &networkSecurityGroup,
							RouteTable:           &routeTable,
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
