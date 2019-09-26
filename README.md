# azk
Create Kubernetes Clusters on Azure, with configurable controllers, providing provisioning and beyond management capabilities

## Principles

- Kubernetes Cluster should be self managed, infrastructure as well as applications
- External sources can only define target behaviour, the cluster strives to reach the target behaviour
- Extensible to provide new behaviours easily
- No additional resources created to achieve target behaviour
- Provide best possible configured kubernetes cluster for desired workload

## Architecture

![](Architecture.jpg)

- azk, a gateway client for cluster management
    - Create
    - Scale
    - Upgrade
    - Renew Certs
    - Deploy addons
    - Delete

## Usage

Either use download the release (relatively stable) or 

```
go get -u github.com/awesomenix/azk
```

Follow the prompts in

```
azk create flow
```

## Workflow

- azk checks for existence of target cluster, using resourcegroupname, subscriptionid
- If the cluster does not exist
    - creates prerequisite resources
        - Generate Certificates
        - Generate Infra management manifests
        - Creates networking base infrastructure
    - bootstraps first master (exactly how HA masters are bootstrapped)
- Deploys cluster infra management components
- Yields control to the in cluster infra management component and waits for its completion
- Supporting controllers will create further changes if required
- Any future operations are directly communicated with the target customer cluster

## Controllers

- __cluster__  
    Manages base infrastructure like  
    - *resource group*  
    - *vnet*
    - *load balancers*  
    - *network security groups*
- __nodeset__  
    Manages set of azure vmss/availability set instances, immutable vm size, immutable kubernetes version
- __nodepool__  
    Manages node sets, if a vm size/kubernetes version changes, creates a rollout for creating new and deprecating old
- __addonmanager__  
    Manages addon list, apply/remove addons listed in addons directory (controller by crd instead of directory)
