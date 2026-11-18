// Package githubapp authenticates as the organization's GitHub App. It
// builds the app JWT, has its SHA-256 digest signed by a Signer (the key file
// mounted from a Secret in the lab, AWS KMS on EKS, chosen in Helm values)
// and exchanges it for installation tokens limited to one repository with
// contents:read, kept in memory until they expire. Callers get a TokenSource
// and never see the key; tokens never reach the renderer.
package githubapp
