package azhelpers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-12-01/network"
	"github.com/Azure/go-autorest/autorest/to"
)

func (c *CloudConfiguration) GetRouteTablesClient() (network.RouteTablesClient, error) {
	routeTableClient := network.NewRouteTablesClient(c.SubscriptionID)
	auth, err := c.getAuthorizerForResource()
	if err != nil {
		return routeTableClient, err
	}
	routeTableClient.Authorizer = auth
	routeTableClient.AddToUserAgent(c.UserAgent)
	return routeTableClient, nil
}

// CreateRouteTables creates a new empty route tables
func (c *CloudConfiguration) CreateRouteTables(ctx context.Context, routeTableName string) (network.RouteTable, error) {
	routeTablesClient, err := c.GetRouteTablesClient()
	if err != nil {
		return network.RouteTable{}, err
	}

	future, err := routeTablesClient.CreateOrUpdate(
		ctx,
		c.GroupName,
		routeTableName,
		network.RouteTable{
			Location:                   to.StringPtr(c.GroupLocation),
			RouteTablePropertiesFormat: &network.RouteTablePropertiesFormat{},
		},
	)

	if err != nil {
		return network.RouteTable{}, fmt.Errorf("cannot create routetable: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, routeTablesClient.Client)
	if err != nil {
		return network.RouteTable{}, fmt.Errorf("cannot get route table create or update future response: %v", err)
	}

	return future.Result(routeTablesClient)
}

// DeleteRouteTables deletes an existing routetable
func (c *CloudConfiguration) DeleteRouteTables(ctx context.Context, routeTableName string) error {
	routeTablesClient, err := c.GetRouteTablesClient()
	if err != nil {
		return err
	}
	future, err := routeTablesClient.Delete(ctx, c.GroupName, routeTableName)
	if err != nil {
		return fmt.Errorf("cannot create route table: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, routeTablesClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get route table create or update future response: %v", err)
	}

	_, err = future.Result(routeTablesClient)
	return err
}
