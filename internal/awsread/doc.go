// Package awsread reads AWS network objects with read-only calls, filtered
// to the VPC and paginated, handling throttling: the AWS kinds of
// model.KeptFields (VPC, subnet, route table, Security Group, Network ACL,
// network interface, NAT gateway, instance, RDS DB instance, EKS cluster,
// node group and the VPC CNI add-on) and nothing else. Each object is
// reduced to its kept fields by model.ReduceBody before it leaves the
// package, which also drops account IDs, ARNs and every tag whose key
// model.Source.KeptTagKeys does not hold.
package awsread
