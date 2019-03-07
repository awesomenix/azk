package azhelpers

import "fmt"

func GetAzureCloudProviderConfig(cloudConfig *CloudConfiguration) string {
	return fmt.Sprintf(`{
"cloud":"AzurePublicCloud",
"tenantId": "%[1]s",
"subscriptionId": "%[2]s",
"aadClientId": "%[3]s",
"aadClientSecret": "%[4]s",
"resourceGroup": "%[5]s",
"location": "%[6]s",
"vmType": "vmss",
"subnetName": "agent-subnet",
"securityGroupName": "azkube-nsg",
"vnetName": "azkube-vnet",
"vnetResourceGroup": "%[5]s",
"routeTableName": "azkube-routetable",
"primaryAvailabilitySetName": "",
"primaryScaleSetName": "",
"cloudProviderBackoff": true,
"cloudProviderBackoffRetries": 6,
"cloudProviderBackoffExponent": 1.5,
"cloudProviderBackoffDuration": 5,
"cloudProviderBackoffJitter": 1.0,
"cloudProviderRatelimit": true,
"cloudProviderRateLimitQPS": 3.0,
"cloudProviderRateLimitBucket": 10,
"useManagedIdentityExtension": false,
"userAssignedIdentityID": "",
"useInstanceMetadata": true,
"loadBalancerSku": "Standard",
"excludeMasterFromStandardLB": true,
"providerVaultName": "",
"maximumLoadBalancerRuleCount": 250,
"providerKeyName": "k8s",
"providerKeyVersion": ""
}`,
		cloudConfig.TenantID,
		cloudConfig.SubscriptionID,
		cloudConfig.ClientID,
		cloudConfig.ClientSecret,
		cloudConfig.GroupName,
		cloudConfig.GroupLocation,
	)
}
