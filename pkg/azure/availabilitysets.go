package azhelpers

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/awesomenix/azkube/pkg/helpers"
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

// CreateVMWithLoadBalancer creates a new VM in an availability set. It also
// creates and configures a load balancer and associates that with the VM's
// NIC.
func (c *CloudConfiguration) CreateVMWithLoadBalancer(
	ctx context.Context,
	vmName,
	lbName,
	vnetName,
	subnetName,
	staticIPAddress,
	customData,
	availabilitySetName string,
	natRule int) error {

	nicName := fmt.Sprintf("nic-%s", vmName)
	//publicipName := fmt.Sprintf("publicip-%s", vmName)

	vmClient, err := c.GetVMClient()
	if err != nil {
		return err
	}

	if _, err := vmClient.Get(ctx, c.GroupName, vmName, ""); err == nil {
		// vm already created
		return nil
	}

	nic, err := c.CreateNICWithLoadBalancer(ctx, lbName, vnetName, subnetName, staticIPAddress, nicName, natRule)
	if err != nil {
		return err
	}

	as, err := c.GetAvailabilitySet(ctx, availabilitySetName)
	if err != nil {
		return err
	}

	var sshKeyData string
	if _, err = os.Stat("/Users/nishp/.ssh/id_rsa.pub"); err == nil {
		sshBytes, err := ioutil.ReadFile("/Users/nishp/.ssh/id_rsa.pub")
		if err != nil {
			log.Fatalf("failed to read SSH key data: %v", err)
		}
		sshKeyData = string(sshBytes)
	}

	future, err := vmClient.CreateOrUpdate(
		ctx,
		c.GroupName,
		vmName,
		compute.VirtualMachine{
			Location: to.StringPtr(c.GroupLocation),
			VirtualMachineProperties: &compute.VirtualMachineProperties{
				HardwareProfile: &compute.HardwareProfile{
					VMSize: compute.VirtualMachineSizeTypesStandardDS2V2,
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
							ID: nic.ID,
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

func (c *CloudConfiguration) AddCustomScriptsExtension(ctx context.Context, vmName, startupScript string) error {
	extensionsClient, err := c.GetVMExtensionsClient()
	if err != nil {
		return err
	}
	future, err := extensionsClient.CreateOrUpdate(
		ctx,
		c.GroupName,
		vmName,
		"startup_script",
		compute.VirtualMachineExtension{
			Name:     to.StringPtr("startup_script"),
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
