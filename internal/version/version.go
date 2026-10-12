// Package version holds the release of the binary.
package version

// Version is the release that this binary is built from, as `git describe`
// gives it: v0.1.0 for a build of that tag, v0.1.0-3-gabc1234 for a build
// three commits later. The Makefile sets it with -ldflags. A build that
// does not use the Makefile, such as a test, has "dev".
var Version = "dev"
