// Package cloudcollector is the cloud-collector service; it implements
// rpc.Collector for aws sources. It reads the AWS network through awsread on
// a schedule and on request, and returns versioned snapshots to the server.
// It never sees pull request content.
package cloudcollector
