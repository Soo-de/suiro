// Package aws implements core.Adapter for AWS network objects. It restates
// them as model gates with AWS semantics: Security Groups are stateful,
// additive allow lists; Network ACLs are stateless, numbered, first-match
// rule lists per direction; route tables decide whether a path exists at
// all. It also emits the topology (subnets, interfaces, NAT gateways), the
// AWS-side endpoints, and the cluster profile values EKS reveals (node
// groups, VPC CNI settings).
//
// Protocol "-1" becomes model.ProtocolAll. IPv6 fields are not kept, so a
// Security Group rule left with no IPv4 range, group or prefix list, a
// Network ACL entry without a CidrBlock and a route without an IPv4
// destination were IPv6 and are dropped, as are ICMP rules; a rule dropped
// that way raises model.ReasonRuleOutsideModel.
package aws
