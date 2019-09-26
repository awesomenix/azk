package azhelpers

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2019-02-01/network"
	"github.com/Azure/go-autorest/autorest/to"
)

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

func (c *CloudConfiguration) CreatePublicIP(ctx context.Context, ipName string) (network.PublicIPAddress, error) {
	ipClient, err := c.GetIPClient()
	if err != nil {
		return network.PublicIPAddress{}, err
	}
	dnsName := fmt.Sprintf("%s.%s.cloudapp.azure.com", strings.ToLower(ipName), strings.ToLower(c.GroupLocation))
	future, err := ipClient.CreateOrUpdate(
		ctx,
		c.GroupName,
		ipName,
		network.PublicIPAddress{
			Sku:      &network.PublicIPAddressSku{Name: network.PublicIPAddressSkuNameStandard},
			Name:     to.StringPtr(ipName),
			Location: to.StringPtr(c.GroupLocation),
			PublicIPAddressPropertiesFormat: &network.PublicIPAddressPropertiesFormat{
				PublicIPAddressVersion:   network.IPv4,
				PublicIPAllocationMethod: network.Static,
				DNSSettings: &network.PublicIPAddressDNSSettings{
					DomainNameLabel: to.StringPtr(strings.ToLower(ipName)),
					Fqdn:            to.StringPtr(dnsName),
				},
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
