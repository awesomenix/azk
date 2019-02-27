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

// GetVirtualNetworkSubnet returns an existing subnet from a virtual network
func (c *CloudConfiguration) GetVirtualNetworkSubnet(ctx context.Context, vnetName, subnetName string) (network.Subnet, error) {
	subnetsClient, err := c.GetSubnetsClient()
	if err != nil {
		return network.Subnet{}, err
	}
	return subnetsClient.Get(ctx, c.GroupName, vnetName, subnetName, "")
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

func (c *CloudConfiguration) GetIPClient() (network.PublicIPAddressesClient, error) {
	ipClient := network.NewPublicIPAddressesClient(c.SubscriptionID)
	auth, err := c.getAuthorizerForResource()
	if err != nil {
		return ipClient, err
	}
	ipClient.Authorizer = auth
	ipClient.AddToUserAgent(c.UserAgent)
	return ipClient, nil
}

// GetPublicIP returns an existing public IP
func (c *CloudConfiguration) GetPublicIP(ctx context.Context, ipName string) (network.PublicIPAddress, error) {
	ipClient, err := c.GetIPClient()
	if err != nil {
		return network.PublicIPAddress{}, err
	}
	return ipClient.Get(ctx, c.GroupName, ipName, "")
}

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

func (c *CloudConfiguration) CreateVirtualNetworkAndSubnets(ctx context.Context, vnetName string) error {
	vnetClient, err := c.GetVNETClient()
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

func (c *CloudConfiguration) CreatePublicIP(ctx context.Context, ipName string) (network.PublicIPAddress, error) {
	ipClient, err := c.GetIPClient()
	if err != nil {
		return network.PublicIPAddress{}, err
	}
	future, err := ipClient.CreateOrUpdate(
		ctx,
		c.GroupName,
		ipName,
		network.PublicIPAddress{
			Name:     to.StringPtr(ipName),
			Location: to.StringPtr(c.GroupLocation),
			PublicIPAddressPropertiesFormat: &network.PublicIPAddressPropertiesFormat{
				PublicIPAddressVersion:   network.IPv4,
				PublicIPAllocationMethod: network.Static,
			},
		},
	)

	if err != nil {
		return network.PublicIPAddress{}, fmt.Errorf("cannot create public ip address: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, ipClient.Client)
	if err != nil {
		return network.PublicIPAddress{}, fmt.Errorf("cannot get public ip address create or update future response: %v", err)
	}

	return future.Result(ipClient)
}

// CreateLoadBalancer creates a load balancer with 2 inbound NAT rules.
func (c *CloudConfiguration) CreateLoadBalancer(ctx context.Context, lbName, pipName string) error {
	probeName := "tcpHTTPSProbe"
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
							FrontendPort:         to.Int32Ptr(443),
							BackendPort:          to.Int32Ptr(6443),
							IdleTimeoutInMinutes: to.Int32Ptr(4),
							EnableFloatingIP:     to.BoolPtr(false),
							LoadDistribution:     network.Default,
							FrontendIPConfiguration: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/frontendIPConfigurations/%s", idPrefix, lbName, frontEndIPConfigName)),
							},
							BackendAddressPool: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/backendAddressPools/%s", idPrefix, lbName, backEndAddressPoolName)),
							},
							Probe: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/probes/%s", idPrefix, lbName, probeName)),
							},
						},
					},
				},
				InboundNatRules: &[]network.InboundNatRule{
					{
						Name: to.StringPtr("natRule1"),
						InboundNatRulePropertiesFormat: &network.InboundNatRulePropertiesFormat{
							Protocol:             network.TransportProtocolTCP,
							FrontendPort:         to.Int32Ptr(22),
							BackendPort:          to.Int32Ptr(22),
							EnableFloatingIP:     to.BoolPtr(false),
							IdleTimeoutInMinutes: to.Int32Ptr(4),
							FrontendIPConfiguration: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/frontendIPConfigurations/%s", idPrefix, lbName, frontEndIPConfigName)),
							},
						},
					},
					{
						Name: to.StringPtr("natRule2"),
						InboundNatRulePropertiesFormat: &network.InboundNatRulePropertiesFormat{
							Protocol:             network.TransportProtocolTCP,
							FrontendPort:         to.Int32Ptr(2201),
							BackendPort:          to.Int32Ptr(22),
							EnableFloatingIP:     to.BoolPtr(false),
							IdleTimeoutInMinutes: to.Int32Ptr(4),
							FrontendIPConfiguration: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/frontendIPConfigurations/%s", idPrefix, lbName, frontEndIPConfigName)),
							},
						},
					},
					{
						Name: to.StringPtr("natRule3"),
						InboundNatRulePropertiesFormat: &network.InboundNatRulePropertiesFormat{
							Protocol:             network.TransportProtocolTCP,
							FrontendPort:         to.Int32Ptr(2202),
							BackendPort:          to.Int32Ptr(22),
							EnableFloatingIP:     to.BoolPtr(false),
							IdleTimeoutInMinutes: to.Int32Ptr(4),
							FrontendIPConfiguration: &network.SubResource{
								ID: to.StringPtr(fmt.Sprintf("/%s/%s/frontendIPConfigurations/%s", idPrefix, lbName, frontEndIPConfigName)),
							},
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

func (c *CloudConfiguration) CreateNICWithLoadBalancer(ctx context.Context, lbName, vnetName, subnetName, staticIPAddress, nicName string, natRule int) (nic network.Interface, err error) {
	subnet, err := c.GetVirtualNetworkSubnet(ctx, vnetName, subnetName)
	if err != nil {
		return
	}

	lb, err := c.GetLoadBalancer(ctx, lbName)
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
