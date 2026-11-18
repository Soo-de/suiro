// Package aws turns AWS network objects (Security Group, Network ACL, route
// table, subnet, network interface) into model gates with AWS semantics:
// Security Groups are stateful allow lists, Network ACLs are stateless
// numbered rule lists evaluated per direction, route tables decide whether a
// path exists at all.
package aws
