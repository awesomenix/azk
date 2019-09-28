package azhelpers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2019-02-01/network"
	"github.com/Azure/go-autorest/autorest/to"
)

func (c *CloudConfiguration) GetLBClient() (network.LoadBalancersClient, error) {
	lbClient := network.NewLoadBalancersClient(c.SubscriptionID)
	auth, err := c.getAuthorizerForResource()
	if err != nil {
		return lbClient, err
	}
	lbClient.Authorizer = auth
	lbClient.AddToUserAgent(c.UserAgent)
	return lbClient, nil
}

// GetLoadBalancer gets info on a loadbalancer
func (c *CloudConfiguration) GetLoadBalancer(ctx context.Context, lbName string) (network.LoadBalancer, error) {
	lbClient, err := c.GetLBClient()
	if err != nil {
		return network.LoadBalancer{}, err
	}
	return lbClient.Get(ctx, c.GroupName, lbName, "")
}

// CreateLoadBalancer creates a load balancer with 2 inbound NAT rules.
func (c *CloudConfiguration) CreateLoadBalancer(ctx context.Context, lbName, pipName string) error {
	probeName := "httpsProbe"
	frontEndIPConfigName := "master-lbFrontEnd"
	backEndAddressPoolName := "master-backEndPool"
	idPrefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers", c.SubscriptionID, c.GroupName)

	pip, err := c.CreatePublicIP(ctx, pipName)
	if err != nil {
		return err
	}

	lbClient, err := c.GetLBClient()
	if err != nil {
		return err
	}
	future, err := lbClient.CreateOrUpdate(ctx,
		c.GroupName,
		lbName,
		network.LoadBalancer{
			Sku:      &network.LoadBalancerSku{Name: network.LoadBalancerSkuNameStandard},
			Location: to.StringPtr(c.GroupLocation),
			LoadBalancerPropertiesFormat: &network.LoadBalancerPropertiesFormat{
				FrontendIPConfigurations: &[]network.FrontendIPConfiguration{
					{
						Name: &frontEndIPConfigName,
						FrontendIPConfigurationPropertiesFormat: &network.FrontendIPConfigurationPropertiesFormat{
							PrivateIPAllocationMethod: network.Dynamic,
							PublicIPAddress:           &pip,
						},
					},
				},
				BackendAddressPools: &[]network.BackendAddressPool{
					{
						Name: &backEndAddressPoolName,
					},
				},
				Probes: &[]network.Probe{
					{
						Name: &probeName,
						ProbePropertiesFormat: &network.ProbePropertiesFormat{
							Protocol:          network.ProbeProtocolHTTPS,
							Port:              to.Int32Ptr(6443),
							RequestPath:       to.StringPtr("/healthz"),
							IntervalInSeconds: to.Int32Ptr(5),
							NumberOfProbes:    to.Int32Ptr(2),
						},
					},
				},
				LoadBalancingRules: &[]network.LoadBalancingRule{
					{
						Name: to.StringPtr("LBRuleHTTPS"),
						LoadBalancingRulePropertiesFormat: &network.LoadBalancingRulePropertiesFormat{
							Protocol:             network.TransportProtocolTCP,
							FrontendPort:         to.Int32Ptr(6443),
							BackendPort:          to.Int32Ptr(6443),
							IdleTimeoutInMinutes: to.Int32Ptr(4),
							EnableFloatingIP:     to.BoolPtr(false),
							LoadDistribution:     network.LoadDistributionDefault,
							FrontendIPConfiguration: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/frontendIPConfigurations/%s", idPrefix, lbName, frontEndIPConfigName)),
							},
							BackendAddressPool: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/backendAddressPools/%s", idPrefix, lbName, backEndAddressPoolName)),
							},
							Probe: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/probes/%s", idPrefix, lbName, probeName)),
							},
							EnableTCPReset: to.BoolPtr(true),
						},
					},
				},
				InboundNatPools: &[]network.InboundNatPool{
					network.InboundNatPool{
						InboundNatPoolPropertiesFormat: &network.InboundNatPoolPropertiesFormat{
							FrontendIPConfiguration: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/frontendIPConfigurations/%s", idPrefix, lbName, frontEndIPConfigName)),
							},
							Protocol:               network.TransportProtocolTCP,
							FrontendPortRangeStart: to.Int32Ptr(2200),
							FrontendPortRangeEnd:   to.Int32Ptr(2210),
							BackendPort:            to.Int32Ptr(22),
							EnableFloatingIP:       to.BoolPtr(false),
							IdleTimeoutInMinutes:   to.Int32Ptr(4),
							EnableTCPReset:         to.BoolPtr(true),
						},
						Name: to.StringPtr("natSSHPool"),
					},
				},
			},
		})

	if err != nil {
		return fmt.Errorf("cannot create load balancer: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, lbClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get load balancer create or update future response: %v", err)
	}

	_, err = future.Result(lbClient)
	return err
}

// CreateLoadBalancer creates a load balancer with 2 inbound NAT rules.
func (c *CloudConfiguration) CreateInternalLoadBalancer(ctx context.Context, vnetName, subnetName, lbName string) error {
	probeName := "tcpHTTPSProbe"
	frontEndIPConfigName := "master-internal-lbFrontEnd"
	backEndAddressPoolName := "master-internal-backEndPool"
	idPrefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers", c.SubscriptionID, c.GroupName)

	subnetClient, err := c.GetSubnetsClient()
	if err != nil {
		return err
	}

	subnet, err := subnetClient.Get(ctx, c.GroupName, vnetName, subnetName, "")
	if err != nil {
		return err
	}

	lbClient, err := c.GetLBClient()
	if err != nil {
		return err
	}
	future, err := lbClient.CreateOrUpdate(ctx,
		c.GroupName,
		lbName,
		network.LoadBalancer{
			Sku:      &network.LoadBalancerSku{Name: network.LoadBalancerSkuNameStandard},
			Location: to.StringPtr(c.GroupLocation),
			LoadBalancerPropertiesFormat: &network.LoadBalancerPropertiesFormat{
				FrontendIPConfigurations: &[]network.FrontendIPConfiguration{
					{
						Name: &frontEndIPConfigName,
						FrontendIPConfigurationPropertiesFormat: &network.FrontendIPConfigurationPropertiesFormat{
							PrivateIPAllocationMethod: network.Static,
							Subnet:                    &subnet,
							PrivateIPAddress:          to.StringPtr("10.0.0.100"),
						},
					},
				},
				BackendAddressPools: &[]network.BackendAddressPool{
					{
						Name: &backEndAddressPoolName,
					},
				},
				Probes: &[]network.Probe{
					{
						Name: &probeName,
						ProbePropertiesFormat: &network.ProbePropertiesFormat{
							Protocol:          network.ProbeProtocolTCP,
							Port:              to.Int32Ptr(6443),
							IntervalInSeconds: to.Int32Ptr(15),
							NumberOfProbes:    to.Int32Ptr(4),
						},
					},
				},
				LoadBalancingRules: &[]network.LoadBalancingRule{
					{
						Name: to.StringPtr("LBRuleHTTPS"),
						LoadBalancingRulePropertiesFormat: &network.LoadBalancingRulePropertiesFormat{
							Protocol:             network.TransportProtocolTCP,
							FrontendPort:         to.Int32Ptr(6443),
							BackendPort:          to.Int32Ptr(6443),
							IdleTimeoutInMinutes: to.Int32Ptr(4),
							EnableFloatingIP:     to.BoolPtr(false),
							LoadDistribution:     network.LoadDistributionDefault,
							FrontendIPConfiguration: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/frontendIPConfigurations/%s", idPrefix, lbName, frontEndIPConfigName)),
							},
							BackendAddressPool: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/backendAddressPools/%s", idPrefix, lbName, backEndAddressPoolName)),
							},
							Probe: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/probes/%s", idPrefix, lbName, probeName)),
							},
							EnableTCPReset: to.BoolPtr(true),
						},
					},
				},
			},
		})

	if err != nil {
		return fmt.Errorf("cannot create load balancer: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, lbClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get load balancer create or update future response: %v", err)
	}

	_, err = future.Result(lbClient)
	return err
}
