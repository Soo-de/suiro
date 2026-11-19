package model

import (
	"maps"
	"slices"
)

// Object kinds the adapters translate or the collectors read.
const (
	KindNamespace     ObjectKind = "k8s:core/Namespace"
	KindPod           ObjectKind = "k8s:core/Pod"
	KindService       ObjectKind = "k8s:core/Service"
	KindDeployment    ObjectKind = "k8s:apps/Deployment"
	KindStatefulSet   ObjectKind = "k8s:apps/StatefulSet"
	KindDaemonSet     ObjectKind = "k8s:apps/DaemonSet"
	KindReplicaSet    ObjectKind = "k8s:apps/ReplicaSet"
	KindJob           ObjectKind = "k8s:batch/Job"
	KindCronJob       ObjectKind = "k8s:batch/CronJob"
	KindNetworkPolicy ObjectKind = "k8s:networking.k8s.io/NetworkPolicy"
	KindIngress       ObjectKind = "k8s:networking.k8s.io/Ingress"

	KindVPC              ObjectKind = "aws:ec2/Vpc"
	KindSubnet           ObjectKind = "aws:ec2/Subnet"
	KindSecurityGroup    ObjectKind = "aws:ec2/SecurityGroup"
	KindNetworkACL       ObjectKind = "aws:ec2/NetworkAcl"
	KindRouteTable       ObjectKind = "aws:ec2/RouteTable"
	KindNetworkInterface ObjectKind = "aws:ec2/NetworkInterface"
	KindNATGateway       ObjectKind = "aws:ec2/NatGateway"
	KindInstance         ObjectKind = "aws:ec2/Instance"
	KindEKSCluster       ObjectKind = "aws:eks/Cluster"
	KindNodegroup        ObjectKind = "aws:eks/Nodegroup"
	KindEKSAddon         ObjectKind = "aws:eks/Addon"

	// KindExpectation is one entry of an application's ExpectationsFile.
	// Its ObjectKey is "<KindExpectation>/<Expectation.ID>".
	KindExpectation ObjectKind = "suiro:core/Expectation"
)

// FieldPath is one kept field in an object's own JSON shape. Segments are
// separated by "." and named as in the source's JSON. "name[]" steps into
// every element of the list "name". "name[Key=value]" keeps only the
// elements of "name" whose Key field equals value (AWS tag lists). A path
// that ends at an object or a list keeps it whole.
//
// Two kept fields are reduced further:
//   - aws:eks/Addon returns ConfigurationValues as a JSON or YAML string.
//     cloud-collector parses it and keeps only the listed sub-paths, as an
//     object under ConfigurationValues.
//   - aws:ec2/NetworkInterface keeps Description only when it has one of the
//     AWS-managed forms the aws adapter classifies interfaces by:
//     "ELB app/<name>/<id>" (Application Load Balancer), "ELB net/<name>/<id>"
//     (Network Load Balancer), "ELB <name>" (Classic Load Balancer) and
//     "RDSNetworkInterface" (RDS), where <name> is 1 to 32 letters, digits
//     and hyphens and <id> is 16 hex digits. cloud-collector drops any other
//     Description: an ECS task's interface carries there an ARN with the
//     account ID, and an interface someone made carries free text.
type FieldPath string

// KubeIdentityFields returns the fields kept for every Kubernetes object of
// any kind, so an object outside KeptFields still shows up by identity (and
// is ignored, flagged or warned about, never silently lost). Labels are not
// part of identity: they are kept only where selectors read them.
func KubeIdentityFields() []FieldPath { return slices.Clone(kubeIdentityFields) }

var kubeIdentityFields = []FieldPath{
	"apiVersion",
	"kind",
	"metadata.name",
	"metadata.namespace",
}

// KeptFields returns the field whitelist entry of kind: the only fields a
// stored object of that kind keeps beyond KubeIdentityFields, and false
// when kind has no entry. The renderer applies the whitelist to every
// rendered object and every expectation entry, and cloud-collector to every
// AWS object, before either leaves its service, so the server, the
// database and the logs never see anything else. The whitelist is
// unexported and read through functions that return a copy, so nothing can
// change it at run time.
//
// What it leaves out on purpose: Secret and ConfigMap data, container env,
// envFrom, command, args and images, annotations (they can hold anything,
// last-applied-configuration included), rule descriptions, AWS tags other
// than the node-group ones, account IDs (OwnerId, UserIdGroupPairs[].UserId,
// RequesterId) and ARNs, which carry account IDs.
//
// The AWS kinds with an entry are also exactly what cloud-collector reads;
// an AWS kind without one is not read at all.
func KeptFields(kind ObjectKind) ([]FieldPath, bool) {
	f, ok := keptFields[kind]
	return slices.Clone(f), ok
}

// KeptKinds returns every kind KeptFields has an entry for, sorted.
func KeptKinds() []ObjectKind { return slices.Sorted(maps.Keys(keptFields)) }

var keptFields = map[ObjectKind][]FieldPath{
	KindNamespace: {"metadata.labels"},
	KindPod: {
		"metadata.labels",
		"spec.hostNetwork",
		"spec.nodeName",
		"spec.nodeSelector",
		"spec.affinity",
		"spec.tolerations",
		"spec.containers[].name",
		"spec.containers[].ports",
	},
	KindDeployment:  podTemplateKept,
	KindStatefulSet: podTemplateKept,
	KindDaemonSet:   podTemplateKept,
	KindReplicaSet:  podTemplateKept,
	KindJob:         podTemplateKept,
	KindCronJob:     cronJobTemplateKept,
	KindService:     {"spec.type", "spec.selector", "spec.ports"},
	// A NetworkPolicy spec is selectors, CIDRs and ports only.
	KindNetworkPolicy: {"spec"},
	KindIngress:       {"spec.ingressClassName"},

	KindVPC: {"VpcId", "CidrBlock", "CidrBlockAssociationSet[].CidrBlock", "IsDefault"},
	KindSubnet: {
		"SubnetId", "VpcId", "CidrBlock", "AvailabilityZone", "MapPublicIpOnLaunch",
	},
	KindSecurityGroup: {
		"GroupId", "GroupName", "VpcId",
		"IpPermissions[].IpProtocol",
		"IpPermissions[].FromPort",
		"IpPermissions[].ToPort",
		"IpPermissions[].IpRanges[].CidrIp",
		"IpPermissions[].PrefixListIds[].PrefixListId",
		"IpPermissions[].UserIdGroupPairs[].GroupId",
		"IpPermissionsEgress[].IpProtocol",
		"IpPermissionsEgress[].FromPort",
		"IpPermissionsEgress[].ToPort",
		"IpPermissionsEgress[].IpRanges[].CidrIp",
		"IpPermissionsEgress[].PrefixListIds[].PrefixListId",
		"IpPermissionsEgress[].UserIdGroupPairs[].GroupId",
	},
	KindNetworkACL: {
		"NetworkAclId", "VpcId", "IsDefault",
		"Associations[].SubnetId",
		"Entries[].RuleNumber",
		"Entries[].Egress",
		"Entries[].Protocol",
		"Entries[].RuleAction",
		"Entries[].CidrBlock",
		"Entries[].PortRange",
	},
	KindRouteTable: {
		"RouteTableId", "VpcId",
		"Associations[].Main",
		"Associations[].SubnetId",
		"Associations[].GatewayId",
		"Routes[].DestinationCidrBlock",
		"Routes[].DestinationPrefixListId",
		"Routes[].GatewayId",
		"Routes[].NatGatewayId",
		"Routes[].TransitGatewayId",
		"Routes[].VpcPeeringConnectionId",
		"Routes[].NetworkInterfaceId",
		"Routes[].InstanceId",
		"Routes[].LocalGatewayId",
		"Routes[].CarrierGatewayId",
		"Routes[].State",
		"Routes[].Origin",
	},
	// Description is kept only in the forms FieldPath lists.
	KindNetworkInterface: {
		"NetworkInterfaceId", "SubnetId", "VpcId",
		"InterfaceType", "RequesterManaged", "Description",
		"Groups[].GroupId",
		"PrivateIpAddresses[].PrivateIpAddress",
		"PrivateIpAddresses[].Association.PublicIp",
		"Ipv4Prefixes[].Ipv4Prefix",
		"Attachment.InstanceId",
	},
	KindNATGateway: {
		"NatGatewayId", "SubnetId", "VpcId", "ConnectivityType", "State",
		"NatGatewayAddresses[].PublicIp",
		"NatGatewayAddresses[].PrivateIp",
	},
	// Instances are read only to map nodes to node groups and to the
	// Security Groups they actually carry.
	KindInstance: {
		"InstanceId", "SubnetId", "VpcId",
		"PrivateIpAddress", "PublicIpAddress",
		"SecurityGroups[].GroupId",
		"NetworkInterfaces[].NetworkInterfaceId",
		"Tags[Key=eks:nodegroup-name]",
		"Tags[Key=eks:cluster-name]",
	},
	KindEKSCluster: {
		"Name", "Version",
		"ResourcesVpcConfig.VpcId",
		"ResourcesVpcConfig.SubnetIds",
		"ResourcesVpcConfig.SecurityGroupIds",
		"ResourcesVpcConfig.ClusterSecurityGroupId",
		"KubernetesNetworkConfig.ServiceIpv4Cidr",
		"KubernetesNetworkConfig.IpFamily",
	},
	KindNodegroup: {
		"NodegroupName", "ClusterName", "Subnets", "Labels", "Taints",
		"Resources.RemoteAccessSecurityGroup",
	},
	// Only the VPC CNI add-on is read. The env names below are VPC CNI
	// settings kept by name; no other environment value is kept.
	KindEKSAddon: {
		"AddonName", "AddonVersion",
		"ConfigurationValues.enableNetworkPolicy",
		"ConfigurationValues.env.NETWORK_POLICY_ENFORCING_MODE",
		"ConfigurationValues.env.AWS_VPC_K8S_CNI_EXTERNALSNAT",
		"ConfigurationValues.env.ENABLE_POD_ENI",
		"ConfigurationValues.env.AWS_VPC_K8S_CNI_CUSTOM_NETWORK_CFG",
		"ConfigurationValues.env.ENABLE_PREFIX_DELEGATION",
	},

	// An expectation keeps the fields of ExpectationSpec and nothing a
	// writer adds beside them.
	KindExpectation: {
		"name", "sources", "destinations", "ports",
		"sourceQuantifier", "destinationQuantifier",
	},
}

// podTemplateKept is the pod template part of KeptFields for every
// controller whose template sits at spec.template.
var podTemplateKept = []FieldPath{
	"spec.template.metadata.labels",
	"spec.template.spec.hostNetwork",
	"spec.template.spec.nodeName",
	"spec.template.spec.nodeSelector",
	"spec.template.spec.affinity",
	"spec.template.spec.tolerations",
	"spec.template.spec.containers[].name",
	"spec.template.spec.containers[].ports",
}

// cronJobTemplateKept is the same for a CronJob, whose template sits one
// level deeper.
var cronJobTemplateKept = []FieldPath{
	"spec.jobTemplate.spec.template.metadata.labels",
	"spec.jobTemplate.spec.template.spec.hostNetwork",
	"spec.jobTemplate.spec.template.spec.nodeName",
	"spec.jobTemplate.spec.template.spec.nodeSelector",
	"spec.jobTemplate.spec.template.spec.affinity",
	"spec.jobTemplate.spec.template.spec.tolerations",
	"spec.jobTemplate.spec.template.spec.containers[].name",
	"spec.jobTemplate.spec.template.spec.containers[].ports",
}

// UnsupportedKinds returns the Kubernetes kinds that change what the
// network allows but are outside the model, each with the reason an
// adapter raises when it meets one; it never skips the object silently.
// They keep identity only, except where KeptFields lists more.
func UnsupportedKinds() map[ObjectKind]ReasonCode { return maps.Clone(unsupportedKinds) }

var unsupportedKinds = map[ObjectKind]ReasonCode{
	"k8s:crd.projectcalico.org/NetworkPolicy":                 ReasonPolicyKindUnsupported,
	"k8s:crd.projectcalico.org/GlobalNetworkPolicy":           ReasonPolicyKindUnsupported,
	"k8s:projectcalico.org/NetworkPolicy":                     ReasonPolicyKindUnsupported,
	"k8s:projectcalico.org/GlobalNetworkPolicy":               ReasonPolicyKindUnsupported,
	"k8s:cilium.io/CiliumNetworkPolicy":                       ReasonPolicyKindUnsupported,
	"k8s:cilium.io/CiliumClusterwideNetworkPolicy":            ReasonPolicyKindUnsupported,
	"k8s:policy.networking.k8s.io/AdminNetworkPolicy":         ReasonPolicyKindUnsupported,
	"k8s:policy.networking.k8s.io/BaselineAdminNetworkPolicy": ReasonPolicyKindUnsupported,
	"k8s:policy.networking.k8s.io/ClusterNetworkPolicy":       ReasonPolicyKindUnsupported,
	KindIngress:                                    ReasonAWSControllerEffect,
	"k8s:elbv2.k8s.aws/TargetGroupBinding":         ReasonAWSControllerEffect,
	"k8s:vpcresources.k8s.aws/SecurityGroupPolicy": ReasonPodSecurityGroups,
	"k8s:crd.k8s.amazonaws.com/ENIConfig":          ReasonCustomNetworking,
}

// IgnoredKinds returns the Kubernetes kinds with no effect on network
// paths. Adapters skip them without a warning because skipping them is the
// design, not an accident. Every Kubernetes kind that is in none of
// KeptFields, UnsupportedKinds and IgnoredKinds raises
// ReasonKindUnrecognized.
func IgnoredKinds() []ObjectKind { return slices.Clone(ignoredKinds) }

var ignoredKinds = []ObjectKind{
	"k8s:core/Secret",
	"k8s:core/ConfigMap",
	"k8s:core/ServiceAccount",
	"k8s:core/PersistentVolumeClaim",
	"k8s:core/LimitRange",
	"k8s:core/ResourceQuota",
	"k8s:rbac.authorization.k8s.io/Role",
	"k8s:rbac.authorization.k8s.io/RoleBinding",
	"k8s:rbac.authorization.k8s.io/ClusterRole",
	"k8s:rbac.authorization.k8s.io/ClusterRoleBinding",
	"k8s:policy/PodDisruptionBudget",
	"k8s:autoscaling/HorizontalPodAutoscaler",
	"k8s:scheduling.k8s.io/PriorityClass",
	"k8s:storage.k8s.io/StorageClass",
	"k8s:apiextensions.k8s.io/CustomResourceDefinition",
	"k8s:networking.k8s.io/IngressClass",
}
