// Package aws implements core.Adapter for AWS network objects. It restates
// them as model gates with AWS semantics: Security Groups are stateful,
// additive allow lists; Network ACLs are stateless, numbered, first-match
// rule lists per direction; route tables decide whether a path exists at
// all. It also emits the topology (subnets, interfaces, NAT gateways), the
// AWS-side endpoints, and the cluster profile values EKS reveals (node
// groups, VPC CNI settings).
package aws
