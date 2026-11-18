// Package gitcollector is the git-collector service; it implements
// rpc.Collector for git sources. It fetches the registered repositories at
// the target branch and at the test merge commit of every open pull request,
// sends each tree to the renderer through rpc.Renderer and returns
// snapshots to the server. It keeps the source list only in memory, has no
// database access, and is the only service that can authenticate to GitHub,
// through a githubapp.TokenSource.
package gitcollector
