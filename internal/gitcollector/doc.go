// Package gitcollector is the git-collector service; it implements
// rpc.Collector for git sources. It fetches the registered repositories at
// the target branch and at the test merge commit of every open pull request,
// sends each tree to the renderer through rpc.Renderer and returns
// snapshots to the server. It renders nothing itself: the source's
// expectations file (model.ExpectationsFile) reaches the snapshot as the
// renderer returns it, so a pull request's snapshot carries the pull
// request's version of it. The one file it reads is a Helm chart's
// Chart.lock, to fetch the pinned dependencies from the allowed chart
// repositories into the tree before rendering, as helm dependency build
// would, because the renderer has no network; a dependency it cannot fetch
// raises model.ReasonChartDependencyUnavailable. It resolves symbolic links
// as rpc.RenderRequest.Files describes. It keeps the source list only in
// memory, has no database access, and is the only service that can
// authenticate to GitHub, through a githubapp.TokenSource.
package gitcollector
