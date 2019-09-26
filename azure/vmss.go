package azhelpers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-03-01/compute"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/awesomenix/azk/helpers"
	"golang.org/x/crypto/ssh"
)

func GetCustomData(customData map[string]string, customRunData map[string]string) string {
	customDataStr := fmt.Sprintf(`
#cloud-config
write_files:
`)
	for customDataKey, customDataValue := range customData {
		customDataStr += fmt.Sprintf(`
- path: %s
  permissions: "0644"
  encoding: base64
  owner: root
  content: |
    %s
`,
			customDataKey,
			base64.StdEncoding.EncodeToString([]byte(customDataValue)))
	}
	for customDataKey, customDataValue := range customRunData {
		customDataStr += fmt.Sprintf(`
- path: %s
  permissions: "0755"
  encoding: base64
  owner: root
  content: |
    %s
`,
			customDataKey,
			base64.StdEncoding.EncodeToString([]byte(customDataValue)))
	}
	customDataStr += fmt.Sprintf(`
runcmd:
`)
	for customDataKey := range customRunData {
		customDataStr += fmt.Sprintf(`
 - sudo %s
`, customDataKey)
	}

	return customDataStr
}

func (c *CloudConfiguration) GetVMSSExtensionsClient() (compute.VirtualMachineScaleSetExtensionsClient, error) {
	extClient := compute.NewVirtualMachineScaleSetExtensionsClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return extClient, err
	}
	extClient.Authorizer = a
	extClient.AddToUserAgent(c.UserAgent)
	return extClient, nil
}

func (c *CloudConfiguration) GetVMSSClient() (compute.VirtualMachineScaleSetsClient, error) {
	vmssClient := compute.NewVirtualMachineScaleSetsClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return vmssClient, err
	}
	vmssClient.Authorizer = a
	vmssClient.AddToUserAgent(c.UserAgent)
	return vmssClient, nil
}

func (c *CloudConfiguration) GetVMSSVMsClient() (compute.VirtualMachineScaleSetVMsClient, error) {
	vmssVMsClient := compute.NewVirtualMachineScaleSetVMsClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return vmssVMsClient, err
	}
	vmssVMsClient.Authorizer = a
	vmssVMsClient.AddToUserAgent(c.UserAgent)
	return vmssVMsClient, nil
}

// CreateVMSS creates a new virtual machine scale set with the specified name using the specified vnet and subnet.
// Username, password, and sshPublicKeyPath determine logon credentials.
func (c *CloudConfiguration) CreateVMSS(ctx context.Context,
	vmssName,
	subnetID string,
	loadbalancerIDs []string,
	natPoolIDs []string,
	//startupScript,
	customData,
	vmSKUType string,
	count int) error {

	var backendAddressPools []compute.SubResource
	for _, loadBalancerID := range loadbalancerIDs {
		backendAddressPools = append(backendAddressPools, compute.SubResource{ID: to.StringPtr(loadBalancerID)})
	}

	var inboundNatPools []compute.SubResource
	for _, natPoolID := range natPoolIDs {
		inboundNatPools = append(inboundNatPools, compute.SubResource{ID: to.StringPtr(natPoolID)})
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

	vmssClient, err := c.GetVMSSClient()
	if err != nil {
		return err
	}

	zones, err := c.getZones(vmSKUType)
	if err != nil {
		return err
	}

	virtualMachineScaleSet := compute.VirtualMachineScaleSet{
		Location: to.StringPtr(c.GroupLocation),
		Sku: &compute.Sku{
			Name:     to.StringPtr(vmSKUType),
			Capacity: to.Int64Ptr(int64(count)),
		},
		VirtualMachineScaleSetProperties: &compute.VirtualMachineScaleSetProperties{
			Overprovision: to.BoolPtr(false),
			UpgradePolicy: &compute.UpgradePolicy{
				Mode: compute.Automatic,
				AutomaticOSUpgradePolicy: &compute.AutomaticOSUpgradePolicy{
					EnableAutomaticOSUpgrade: to.BoolPtr(false),
				},
			},
			VirtualMachineProfile: &compute.VirtualMachineScaleSetVMProfile{
				OsProfile: &compute.VirtualMachineScaleSetOSProfile{
					ComputerNamePrefix: to.StringPtr(vmssName),
					AdminUsername:      to.StringPtr("azureuser"),
					AdminPassword:      to.StringPtr(helpers.GenerateRandomHexString(32)),
					CustomData:         to.StringPtr(customData),
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
				StorageProfile: &compute.VirtualMachineScaleSetStorageProfile{
					ImageReference: &compute.ImageReference{
						Offer:     to.StringPtr("UbuntuServer"),
						Publisher: to.StringPtr("Canonical"),
						Sku:       to.StringPtr("18.04-LTS"),
						Version:   to.StringPtr("latest"),
					},
					OsDisk: &compute.VirtualMachineScaleSetOSDisk{
						CreateOption: compute.DiskCreateOptionTypesFromImage,
						DiskSizeGB:   to.Int32Ptr(64),
						OsType:       compute.Linux,
						ManagedDisk: &compute.VirtualMachineScaleSetManagedDiskParameters{
							StorageAccountType: compute.StorageAccountTypesPremiumLRS,
						},
					},
				},
				NetworkProfile: &compute.VirtualMachineScaleSetNetworkProfile{
					NetworkInterfaceConfigurations: &[]compute.VirtualMachineScaleSetNetworkConfiguration{
						{
							Name: to.StringPtr(vmssName),
							VirtualMachineScaleSetNetworkConfigurationProperties: &compute.VirtualMachineScaleSetNetworkConfigurationProperties{
								Primary:            to.BoolPtr(true),
								EnableIPForwarding: to.BoolPtr(true),
								IPConfigurations: &[]compute.VirtualMachineScaleSetIPConfiguration{
									{
										Name: to.StringPtr(vmssName),
										VirtualMachineScaleSetIPConfigurationProperties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
											Subnet: &compute.APIEntityReference{
												ID: to.StringPtr(subnetID),
											},
											LoadBalancerBackendAddressPools: &backendAddressPools,
											LoadBalancerInboundNatPools:     &inboundNatPools,
										},
									},
								},
							},
						},
					},
				},
				// ExtensionProfile: &compute.VirtualMachineScaleSetExtensionProfile{
				// 	Extensions: &[]compute.VirtualMachineScaleSetExtension{
				// 		{
				// 			Name: to.StringPtr("startup_script"),
				// 			VirtualMachineScaleSetExtensionProperties: &compute.VirtualMachineScaleSetExtensionProperties{
				// 				Type:                    to.StringPtr("CustomScript"),
				// 				TypeHandlerVersion:      to.StringPtr("2.0"),
				// 				AutoUpgradeMinorVersion: to.BoolPtr(true),
				// 				Settings:                map[string]bool{"skipDos2Unix": true},
				// 				Publisher:               to.StringPtr("Microsoft.Azure.Extensions"),
				// 				ProtectedSettings:       map[string]string{"script": startupScript},
				// 			},
				// 		},
				// 	},
				// },
			},
		},
	}

	if zones != nil && len(zones) > 0 {
		virtualMachineScaleSet.Zones = &zones
		virtualMachineScaleSet.VirtualMachineScaleSetProperties.ZoneBalance = to.BoolPtr(true)
	}

	future, err := vmssClient.CreateOrUpdate(
		ctx,
		c.GroupName,
		vmssName,
		virtualMachineScaleSet,
	)
	if err != nil {
		return fmt.Errorf("cannot create vmss: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, vmssClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get the vmss create or update future response: %v", err)
	}

	_, err = future.Result(vmssClient)
	return err
}

// GetVMSS gets the specified VMSS info
func (c *CloudConfiguration) GetVMSS(ctx context.Context, vmssName string) (compute.VirtualMachineScaleSet, error) {
	vmssClient, err := c.GetVMSSClient()
	if err != nil {
		return compute.VirtualMachineScaleSet{}, err
	}
	return vmssClient.Get(ctx, c.GroupName, vmssName)
}

// UpdateVMSS modifies the VMSS resource by getting it, updating it locally, and
// putting it back to the server.
// func (c *CloudConfiguration) UpdateVMSS(ctx context.Context, vmssName string, tags map[string]*string) (vmss compute.VirtualMachineScaleSet, err error) {

// 	// get the VMSS resource
// 	vmss, err = GetVMSS(ctx, vmssName)
// 	if err != nil {
// 		return
// 	}

// 	// update it
// 	vmss.Tags = tags

// 	// PUT it back
// 	vmssClient := getVMSSClient()
// 	future, err := vmssClient.CreateOrUpdate(ctx, groupName, vmssName, vmss)
// 	if err != nil {
// 		return vmss, fmt.Errorf("cannot update vmss: %v", err)
// 	}

// 	err = future.WaitForCompletionRef(ctx, vmssClient.Client)
// 	if err != nil {
// 		return vmss, fmt.Errorf("cannot get the vmss create or update future response: %v", err)
// 	}

// 	return future.Result(vmssClient)
// }

func (c *CloudConfiguration) ScaleVMSS(ctx context.Context, vmssName string, customData string, count int) error {
	vmssClient, err := c.GetVMSSClient()
	if err != nil {
		return err
	}

	vmss, err := vmssClient.Get(ctx, c.GroupName, vmssName)
	if err != nil {
		return fmt.Errorf("cannot update vmss: %v", err)
	}

	if *vmss.Sku.Capacity == int64(count) {
		return nil
	}

	// passing nil instance ids will deallocate all VMs in the VMSS
	future, err := vmssClient.Update(ctx, c.GroupName, vmssName, compute.VirtualMachineScaleSetUpdate{
		Sku: &compute.Sku{
			Capacity: to.Int64Ptr(int64(count)),
		},
		VirtualMachineScaleSetUpdateProperties: &compute.VirtualMachineScaleSetUpdateProperties{
			VirtualMachineProfile: &compute.VirtualMachineScaleSetUpdateVMProfile{
				OsProfile: &compute.VirtualMachineScaleSetUpdateOSProfile{
					CustomData: to.StringPtr(customData),
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("cannot update vmss: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, vmssClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get the vmss update future response: %v", err)
	}

	_, err = future.Result(vmssClient)
	return err
}

// DeleteVMSS deallocates the selected VMSS
func (c *CloudConfiguration) DeleteVMSS(ctx context.Context, vmssName string) error {
	vmssClient, err := c.GetVMSSClient()
	// passing nil instance ids will deallocate all VMs in the VMSS
	future, err := vmssClient.Delete(ctx, c.GroupName, vmssName)
	if err != nil {
		return fmt.Errorf("cannot delete vmss: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, vmssClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get the vmss deallocate future response: %v", err)
	}

	_, err = future.Result(vmssClient)
	return err
}

func (c *CloudConfiguration) getZones(vmSKU string) ([]string, error) {
	resClient := compute.NewResourceSkusClient(c.SubscriptionID)
	a, err := c.getAuthorizerForResource()
	if err != nil {
		return nil, err
	}
	resClient.Authorizer = a
	resClient.AddToUserAgent(c.UserAgent)

	res, err := resClient.List(context.TODO())
	if err != nil {
		return nil, err
	}

	var zones []string
	for _, resSku := range res.Values() {
		if !strings.EqualFold(*resSku.Name, vmSKU) {
			continue
		}
		for _, locationInfo := range *resSku.LocationInfo {
			if !strings.EqualFold(*locationInfo.Location, c.GroupLocation) {
				continue
			}
			zones = *locationInfo.Zones
		}

	}
	return zones, nil
}

// StartVMSS starts the selected VMSS
// func (c *CloudConfiguration) StartVMSS(ctx context.Context, vmssName string) (osr autorest.Response, err error) {
// 	vmssClient := getVMSSClient()
// 	// passing nil instance ids will start all VMs in the VMSS
// 	future, err := vmssClient.Start(ctx, groupName, vmssName, nil)
// 	if err != nil {
// 		return osr, fmt.Errorf("cannot start vmss: %v", err)
// 	}

// 	err = future.WaitForCompletionRef(ctx, vmssClient.Client)
// 	if err != nil {
// 		return osr, fmt.Errorf("cannot get the vmss start future response: %v", err)
// 	}

// 	return future.Result(vmssClient)
// }

// // RestartVMSS restarts the selected VMSS
// func (c *CloudConfiguration) RestartVMSS(ctx context.Context, vmssName string) (osr autorest.Response, err error) {
// 	vmssClient := getVMSSClient()
// 	// passing nil instance ids will restart all VMs in the VMSS
// 	future, err := vmssClient.Restart(ctx, groupName, vmssName, nil)
// 	if err != nil {
// 		return osr, fmt.Errorf("cannot restart vm: %v", err)
// 	}

// 	err = future.WaitForCompletionRef(ctx, vmssClient.Client)
// 	if err != nil {
// 		return osr, fmt.Errorf("cannot get the vm restart future response: %v", err)
// 	}

// 	return future.Result(vmssClient)
// }

// // StopVMSS stops the selected VMSS
// func StopVMSS(ctx context.Context, vmssName string) (osr autorest.Response, err error) {
// 	vmssClient := getVMSSClient()
// 	// passing nil instance ids will stop all VMs in the VMSS
// 	future, err := vmssClient.PowerOff(ctx, groupName, vmssName, nil)
// 	if err != nil {
// 		return osr, fmt.Errorf("cannot power off vmss: %v", err)
// 	}

// 	err = future.WaitForCompletionRef(ctx, vmssClient.Client)
// 	if err != nil {
// 		return osr, fmt.Errorf("cannot get the vmss power off future response: %v", err)
// 	}

// 	return future.Result(vmssClient)
// }
