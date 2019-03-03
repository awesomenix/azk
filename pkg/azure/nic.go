package azhelpers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-01-01/network"
	"github.com/Azure/go-autorest/autorest/to"
)

func (c *CloudConfiguration) GetNICClient() (network.InterfacesClient, error) {
	nicClient := network.NewInterfacesClient(c.SubscriptionID)
	auth, err := c.getAuthorizerForResource()
	if err != nil {
		return nicClient, err
	}
	nicClient.Authorizer = auth
	nicClient.AddToUserAgent(c.UserAgent)
	return nicClient, nil
}

func (c *CloudConfiguration) CreateNICWithLoadBalancer(ctx context.Context, lbName, internallbName, vnetName, subnetName, staticIPAddress, nicName string, natRule int) (nic network.Interface, err error) {
	subnet, err := c.GetSubnet(ctx, vnetName, subnetName)
	if err != nil {
		return
	}

	lb, err := c.GetLoadBalancer(ctx, lbName)
	if err != nil {
		return
	}

	internallb, err := c.GetLoadBalancer(ctx, internallbName)
	if err != nil {
		return
	}

	nicClient, err := c.GetNICClient()
	if err != nil {
		return network.Interface{}, err
	}
	future, err := nicClient.CreateOrUpdate(ctx,
		c.GroupName,
		nicName,
		network.Interface{
			Location: to.StringPtr(c.GroupLocation),
			InterfacePropertiesFormat: &network.InterfacePropertiesFormat{
				IPConfigurations: &[]network.InterfaceIPConfiguration{
					{
						Name: to.StringPtr("pipConfig"),
						InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
							Subnet: &network.Subnet{
								ID: subnet.ID,
							},
							PrivateIPAllocationMethod: network.IPAllocationMethod("Static"),
							PrivateIPAddress:          to.StringPtr(staticIPAddress),
							LoadBalancerBackendAddressPools: &[]network.BackendAddressPool{
								{
									ID: (*lb.BackendAddressPools)[0].ID,
								},
								{
									ID: (*internallb.BackendAddressPools)[0].ID,
								},
							},
							LoadBalancerInboundNatRules: &[]network.InboundNatRule{
								{
									ID: (*lb.InboundNatRules)[natRule].ID,
								},
							},
						},
					},
				},
			},
		})
	if err != nil {
		return nic, fmt.Errorf("cannot create nic: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, nicClient.Client)
	if err != nil {
		return nic, fmt.Errorf("cannot get nic create or update future response: %v", err)
	}

	return future.Result(nicClient)
}
