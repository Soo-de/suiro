// Package cloudcollector is the cloud-collector service; it implements
// rpc.Collector for aws and cluster sources. It reads the AWS network
// through awsread, and the Namespace objects of the cluster it runs in
// through a ClusterRole that allows list and watch on namespaces and
// nothing else, both on a schedule and on request, and returns versioned
// snapshots to the server. It never sees pull request content.
package cloudcollector
