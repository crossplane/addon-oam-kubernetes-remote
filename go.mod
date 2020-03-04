module github.com/crossplane/addon-oam-kubernetes-remote

go 1.13

require (
	github.com/crossplane/crossplane v0.8.0-rc.0.20200306224957-b724d2eba282
	github.com/crossplane/crossplane-runtime v0.5.1-0.20200304190937-e98175fed978
	github.com/google/go-cmp v0.3.1
	github.com/pkg/errors v0.8.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/client-go v0.17.3
	sigs.k8s.io/controller-runtime v0.4.0
)
