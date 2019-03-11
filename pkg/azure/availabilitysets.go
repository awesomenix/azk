package azhelpers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/awesomenix/azk/pkg/helpers"
	"golang.org/x/crypto/ssh"
)

func (c *CloudConfiguration) GetAvailabilitySetsClient() (compute.AvailabilitySetsClient, error) {
	asClient := compute.NewAvailabilitySetsClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return asClient, err
	}
	asClient.Authorizer = a
	asClient.AddToUserAgent(c.UserAgent)
	return asClient, nil
}

func (c *CloudConfiguration) GetVMClient() (compute.VirtualMachinesClient, error) {
	vmClient := compute.NewVirtualMachinesClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return vmClient, err
	}
	vmClient.Authorizer = a
	vmClient.AddToUserAgent(c.UserAgent)
	return vmClient, nil
}

func (c *CloudConfiguration) GetVMExtensionsClient() (compute.VirtualMachineExtensionsClient, error) {
	extClient := compute.NewVirtualMachineExtensionsClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return extClient, err
	}
	extClient.Authorizer = a
	extClient.AddToUserAgent(c.UserAgent)
	return extClient, nil
}

// CreateAvailabilitySet creates an availability set
func (c *CloudConfiguration) CreateAvailabilitySet(ctx context.Context, asName string) (compute.AvailabilitySet, error) {
	asClient, err := c.GetAvailabilitySetsClient()
	if err != nil {
		return compute.AvailabilitySet{}, err
	}
	return asClient.CreateOrUpdate(ctx,
		c.GroupName,
		asName,
		compute.AvailabilitySet{
			Location: to.StringPtr(c.GroupLocation),
			AvailabilitySetProperties: &compute.AvailabilitySetProperties{
				PlatformFaultDomainCount:  to.Int32Ptr(2),
				PlatformUpdateDomainCount: to.Int32Ptr(3),
			},
			Sku: &compute.Sku{
				Name: to.StringPtr("Aligned"),
			},
		})
}

// GetAvailabilitySet gets info on an availability set
func (c *CloudConfiguration) GetAvailabilitySet(ctx context.Context, asName string) (compute.AvailabilitySet, error) {
	asClient, err := c.GetAvailabilitySetsClient()
	if err != nil {
		return compute.AvailabilitySet{}, err
	}
	return asClient.Get(ctx, c.GroupName, asName)
}

// CreateVMWithNIC creates a new VM in an availability set. It also
// creates and configures the VM's NIC.
func (c *CloudConfiguration) CreateVMWithNIC(
	ctx context.Context,
	vmName,
	vnetName,
	subnetName,
	staticIPAddress,
	customData,
	availabilitySetName,
	vmSKUType string) error {

	nic, err := c.CreateNIC(ctx, vnetName, subnetName, staticIPAddress, fmt.Sprintf("nic-%s", vmName))
	if err != nil {
		return err
	}

	return c.CreateVMInAvailabilitySet(
		ctx,
		vmName,
		availabilitySetName,
		vmSKUType,
		*nic.ID,
		customData,
	)
}

// CreateVMWithLoadBalancer creates a new VM in an availability set. It also
// creates and configures a load balancer and associates that with the VM's
// NIC.
func (c *CloudConfiguration) CreateVMWithLoadBalancer(
	ctx context.Context,
	vmName,
	lbName,
	internallbName,
	vnetName,
	subnetName,
	staticIPAddress,
	customData,
	availabilitySetName,
	vmSKUType string,
	natRule int) error {

	nic, err := c.CreateNICWithLoadBalancer(ctx, lbName, internallbName, vnetName, subnetName, staticIPAddress, fmt.Sprintf("nic-%s", vmName), natRule)
	if err != nil {
		return err
	}

	return c.CreateVMInAvailabilitySet(
		ctx,
		vmName,
		availabilitySetName,
		vmSKUType,
		*nic.ID,
		customData,
	)
}

func (c *CloudConfiguration) CreateVMInAvailabilitySet(
	ctx context.Context,
	vmName,
	availabilitySetName,
	vmSKUType,
	nicID,
	customData string) error {
	vmClient, err := c.GetVMClient()
	if err != nil {
		return err
	}

	if _, err := vmClient.Get(ctx, c.GroupName, vmName, ""); err == nil {
		// VM already exists
		return nil
	}

	as, err := c.GetAvailabilitySet(ctx, availabilitySetName)
	if err != nil {
		return err
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	publicRsaKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return err
	}

	sshKeyData := string(ssh.MarshalAuthorizedKey(publicRsaKey))

	future, err := vmClient.CreateOrUpdate(
		ctx,
		c.GroupName,
		vmName,
		compute.VirtualMachine{
			Location: to.StringPtr(c.GroupLocation),
			VirtualMachineProperties: &compute.VirtualMachineProperties{
				HardwareProfile: &compute.HardwareProfile{
					VMSize: compute.VirtualMachineSizeTypes(vmSKUType),
				},
				StorageProfile: &compute.StorageProfile{
					ImageReference: &compute.ImageReference{
						Publisher: to.StringPtr("Canonical"),
						Offer:     to.StringPtr("UbuntuServer"),
						Sku:       to.StringPtr("18.04-LTS"),
						Version:   to.StringPtr("latest"),
					},
					OsDisk: &compute.OSDisk{
						CreateOption: compute.DiskCreateOptionTypesFromImage,
						DiskSizeGB:   to.Int32Ptr(64),
					},
				},
				OsProfile: &compute.OSProfile{
					ComputerName:  to.StringPtr(vmName),
					AdminUsername: to.StringPtr("azureuser"),
					AdminPassword: to.StringPtr(helpers.GenerateRandomHexString(32)),
					CustomData:    to.StringPtr(customData),
					LinuxConfiguration: &compute.LinuxConfiguration{
						SSH: &compute.SSHConfiguration{
							PublicKeys: &[]compute.SSHPublicKey{
								{
									Path:    to.StringPtr("/home/azureuser/.ssh/authorized_keys"),
									KeyData: to.StringPtr(sshKeyData),
								},
							},
						},
					},
				},
				NetworkProfile: &compute.NetworkProfile{
					NetworkInterfaces: &[]compute.NetworkInterfaceReference{
						{
							ID: to.StringPtr(nicID),
							NetworkInterfaceReferenceProperties: &compute.NetworkInterfaceReferenceProperties{
								Primary: to.BoolPtr(true),
							},
						},
					},
				},
				AvailabilitySet: &compute.SubResource{
					ID: as.ID,
				},
			},
		})
	if err != nil {
		return fmt.Errorf("cannot create vm: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, vmClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get the vm create or update future response: %v", err)
	}

	_, err = future.Result(vmClient)
	return err
}

func (c *CloudConfiguration) AddCustomScriptsExtension(ctx context.Context, vmName, scriptName, startupScript string) error {
	extensionsClient, err := c.GetVMExtensionsClient()
	if err != nil {
		return err
	}
	future, err := extensionsClient.CreateOrUpdate(
		ctx,
		c.GroupName,
		vmName,
		scriptName,
		compute.VirtualMachineExtension{
			Name:     to.StringPtr(scriptName),
			Location: to.StringPtr(c.GroupLocation),
			VirtualMachineExtensionProperties: &compute.VirtualMachineExtensionProperties{
				Type:                    to.StringPtr("CustomScript"),
				TypeHandlerVersion:      to.StringPtr("2.0"),
				AutoUpgradeMinorVersion: to.BoolPtr(true),
				Settings:                map[string]bool{"skipDos2Unix": true},
				Publisher:               to.StringPtr("Microsoft.Azure.Extensions"),
				ProtectedSettings:       map[string]string{"script": startupScript},
			},
		})
	if err != nil {
		return fmt.Errorf("cannot create vm extension: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, extensionsClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get the extension create or update future response: %v", err)
	}

	_, err = future.Result(extensionsClient)
	return err
}

func (c *CloudConfiguration) DeleteCustomScriptsExtension(ctx context.Context, vmName, scriptName string) error {
	extensionsClient, err := c.GetVMExtensionsClient()
	if err != nil {
		return err
	}
	future, err := extensionsClient.Delete(
		ctx,
		c.GroupName,
		vmName,
		scriptName)
	if err != nil {
		return fmt.Errorf("cannot delete vm extension: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, extensionsClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get the extension delete future response: %v", err)
	}

	_, err = future.Result(extensionsClient)
	return err
}

func (c *CloudConfiguration) DeleteResources(ctx context.Context, resourceName string) error {
	resourcesClient, err := c.GetResourcesClient()
	if err != nil {
		return err
	}

	for i := 0; i < 5; i++ {
		deleteCallback := func(ctx context.Context, resource resources.GenericResource) error {
			if strings.Contains(strings.ToLower(*resource.Name), strings.ToLower(resourceName)) {
				if strings.Contains(strings.ToLower(*resource.Type), strings.ToLower("scaleset")) {
					return nil
				} else if strings.Contains(strings.ToLower(*resource.Type), strings.ToLower("virtualmachines")) {
					//log.Info("Deleting", "VM", *resource.Name)

					vmClient, err := c.GetVMClient()
					if err != nil {
						return err
					}
					future, err := vmClient.Delete(ctx, c.GroupName, *resource.Name)
					if err != nil {
						return err
					}
					future.WaitForCompletionRef(ctx, vmClient.Client)
				} else if strings.Contains(strings.ToLower(*resource.Type), strings.ToLower("availabilityset")) {
					//log.Info("Deleting", "AvailabilitySet", *resource.Name)
					asClient, err := c.GetAvailabilitySetsClient()
					if err != nil {
						return err
					}
					_, err = asClient.Delete(ctx, c.GroupName, *resource.Name)
					if err != nil {
						return err
					}
				} else if strings.Contains(strings.ToLower(*resource.Type), strings.ToLower("disk")) {
					//log.Info("Deleting", "Disk", *resource.Name)
					disksClient, err := c.GetDisksClient()
					if err != nil {
						return err
					}

					future, err := disksClient.Delete(ctx, c.GroupName, *resource.Name)
					if err != nil {
						return err
					}
					future.WaitForCompletionRef(ctx, disksClient.Client)
				} else {
					_, err := resourcesClient.Delete(ctx, c.GroupName, "", "", *resource.Type, *resource.Name)
					return err
				}
			}
			return nil
		}

		err = listResources(ctx, resourcesClient, c.GroupName, deleteCallback)
		time.Sleep(3 * time.Second)
	}

	return err
}

func listResources(ctx context.Context, resourcesClient resources.Client, resourceGroupName string, callback func(ctx context.Context, resource resources.GenericResource) error) error {
	resourcesList, err := resourcesClient.ListByResourceGroup(ctx, resourceGroupName, "", "", nil)
	if err != nil {
		return err
	}
	var topErr error
	topErr = nil
	for _, resource := range resourcesList.Values() {
		if err := callback(ctx, resource); err != nil {
			topErr = err
		}
	}

	return topErr
}
