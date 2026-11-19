// Package terraform will implement core.Adapter for the network fields of a
// Terraform plan JSON, producing the same gates as the aws adapter, so a
// pull request's planned AWS change can be analysed before it is applied.
// It never runs Terraform itself: state carries secrets and plan code can
// execute.
package terraform
