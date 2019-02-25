package azhelpers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2017-05-10/resources"
	"github.com/Azure/go-autorest/autorest/to"
)

func (c *CloudConfiguration) GetGroupsClient() (resources.GroupsClient, error) {
	groupsClient := resources.NewGroupsClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return groupsClient, err
	}
	groupsClient.Authorizer = a
	groupsClient.AddToUserAgent(c.UserAgent)
	return groupsClient, nil
}

func (c *CloudConfiguration) CreateOrUpdateResourceGroup(ctx context.Context, groupName, groupLocation string) error {
	groupsClient, err := c.GetGroupsClient()
	if err != nil {
		return err
	}

	_, err = groupsClient.CreateOrUpdate(ctx, groupName, resources.Group{Location: to.StringPtr(groupLocation)})
	return err
}

func (c *CloudConfiguration) DeleteResourceGroup(ctx context.Context, groupName string) error {
	groupsClient, err := c.GetGroupsClient()
	if err != nil {
		return err
	}

	future, err := groupsClient.Delete(ctx, groupName)
	if err != nil {
		return err
	}

	err = future.WaitForCompletionRef(ctx, groupsClient.Client)
	if err != nil {
		return fmt.Errorf("cannot delete, future response: %v", err)
	}

	_, err = future.Result(groupsClient)

	return err
}
