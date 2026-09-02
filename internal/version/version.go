package version

// Version is the semantic version of pw. Overridable at build time via:
//
//	go build -ldflags "-X pw/internal/version.Version=1.2.3"
var Version = "0.5.1"
