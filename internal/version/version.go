// Package version holds the build-time version string reported by the CLI and
// the health endpoint. The release workflow stamps the exact release tag at
// build time via
//
//	-ldflags "-X github.com/djdembeck/annalist/internal/version.Version=${VERSION}"
//
// where VERSION comes from an `ARG VERSION` in the Dockerfile. The default
// keeps un-stamped dev/CI builds from inventing a version.
package version

// Version is the annalist version, overridable at build time with -ldflags -X.
var Version = "0.1.0"
