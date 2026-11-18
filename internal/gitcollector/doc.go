// Package gitcollector is the git-collector service; it implements
// rpc.Collector for git sources. It fetches the registered repositories at
// the target branch and at the test merge commit of every open pull request,
// sends each tree to the renderer through rpc.Renderer and returns
// snapshots to the server. It parses no repository content itself: the
// source's expectations file (model.ExpectationsFile) reaches the snapshot
// as the renderer returns it, so a pull request's snapshot carries the pull
// request's version of it. It keeps the source list only in memory, has no
// database access, and is the only service that can authenticate to GitHub,
// through a githubapp.TokenSource.
package gitcollector
