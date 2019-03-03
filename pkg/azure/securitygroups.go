package azhelpers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-01-01/network"
	"github.com/Azure/go-autorest/autorest/to"
)

func (c *CloudConfiguration) GetNSGClient() (network.SecurityGroupsClient, error) {
	nsgClient := network.NewSecurityGroupsClient(c.SubscriptionID)
	auth, err := c.getAuthorizerForResource()
	if err != nil {
		return nsgClient, err
	}
	nsgClient.Authorizer = auth
	nsgClient.AddToUserAgent(c.UserAgent)
	return nsgClient, nil
}

// CreateNetworkSecurityGroup creates a new network security group with rules set for allowing SSH and HTTPS use
func (c *CloudConfiguration) CreateNetworkSecurityGroup(ctx context.Context, nsgName string) (network.SecurityGroup, error) {
	nsgClient, err := c.GetNSGClient()
	if err != nil {
		return network.SecurityGroup{}, err
	}

	future, err := nsgClient.CreateOrUpdate(
		ctx,
		c.GroupName,
		nsgName,
		network.SecurityGroup{
			Location: to.StringPtr(c.GroupLocation),
			SecurityGroupPropertiesFormat: &network.SecurityGroupPropertiesFormat{
				SecurityRules: &[]network.SecurityRule{
					{
						Name: to.StringPtr("allow_ssh"),
						SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
							Protocol:                 network.SecurityRuleProtocolTCP,
							SourceAddressPrefix:      to.StringPtr("*"),
							SourcePortRange:          to.StringPtr("*"),
							DestinationAddressPrefix: to.StringPtr("*"),
							DestinationPortRange:     to.StringPtr("22"),
							Access:                   network.SecurityRuleAccessAllow,
							Direction:                network.SecurityRuleDirectionInbound,
							Priority:                 to.Int32Ptr(100),
						},
					},
					// {
					// 	Name: to.StringPtr("allow_https"),
					// 	SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
					// 		Protocol:                 network.SecurityRuleProtocolTCP,
					// 		SourceAddressPrefix:      to.StringPtr("0.0.0.0/0"),
					// 		SourcePortRange:          to.StringPtr("1-65535"),
					// 		DestinationAddressPrefix: to.StringPtr("0.0.0.0/0"),
					// 		DestinationPortRange:     to.StringPtr("443"),
					// 		Access:                   network.SecurityRuleAccessAllow,
					// 		Direction:                network.SecurityRuleDirectionInbound,
					// 		Priority:                 to.Int32Ptr(200),
					// 	},
					// },
				},
			},
		},
	)

	if err != nil {
		return network.SecurityGroup{}, fmt.Errorf("cannot create nsg: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, nsgClient.Client)
	if err != nil {
		return network.SecurityGroup{}, fmt.Errorf("cannot get nsg create or update future response: %v", err)
	}

	return future.Result(nsgClient)
}

//CreateDefaultNetworkSecurityGroup creates a new network security group, without rules (rules can be set later)
func (c *CloudConfiguration) CreateDefaultNetworkSecurityGroup(ctx context.Context, nsgName string) (network.SecurityGroup, error) {
	nsgClient, err := c.GetNSGClient()
	if err != nil {
		return network.SecurityGroup{}, err
	}
	future, err := nsgClient.CreateOrUpdate(
		ctx,
		c.GroupName,
		nsgName,
		network.SecurityGroup{
			Location:                      to.StringPtr(c.GroupLocation),
			SecurityGroupPropertiesFormat: &network.SecurityGroupPropertiesFormat{},
		},
	)

	if err != nil {
		return network.SecurityGroup{}, fmt.Errorf("cannot create nsg: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, nsgClient.Client)
	if err != nil {
		return network.SecurityGroup{}, fmt.Errorf("cannot get nsg create or update future response: %v", err)
	}

	return future.Result(nsgClient)
}

// DeleteNetworkSecurityGroup deletes an existing network security group
func (c *CloudConfiguration) DeleteNetworkSecurityGroup(ctx context.Context, nsgName string) error {
	nsgClient, err := c.GetNSGClient()
	if err != nil {
		return err
	}
	future, err := nsgClient.Delete(ctx, c.GroupName, nsgName)
	if err != nil {
		return fmt.Errorf("cannot create nsg: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, nsgClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get nsg create or update future response: %v", err)
	}

	_, err = future.Result(nsgClient)
	return err
}
