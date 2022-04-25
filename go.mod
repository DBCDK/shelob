module github.com/dbcdk/shelob

go 1.16

require (
	github.com/kavu/go_reuseport v1.5.0
	github.com/prometheus/client_golang v1.12.1
	github.com/sirupsen/logrus v1.8.1
	github.com/viki-org/dnscache v0.0.0-20130720023526-c70c1f23c5d8
	github.com/vulcand/oxy v1.3.0
	go.uber.org/zap v1.21.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	k8s.io/api v0.22.9
	k8s.io/apimachinery v0.22.9
	k8s.io/client-go v0.22.9
)

replace github.com/vulcand/oxy => github.com/ldez/oxy v0.0.0-20210816022403-7ee63b416b8f
