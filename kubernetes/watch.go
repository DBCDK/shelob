package kubernetes

import (
	"fmt"
	"github.com/dbcdk/shelob/util"
	"go.uber.org/zap"
	v13 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

func WatchBackends(config *util.Config, updateChan chan util.Reload) error {
	clients, err := GetKubeClient(config.Kubeconfig)
	if err != nil {
		return err
	}

	emptyOptions := v1.ListOptions{}

	ingressWatch, err := clients.ExtensionsV1beta1().Ingresses("").Watch(emptyOptions)
	if err != nil {
		return err
	}
	defer ingressWatch.Stop()

	serviceWatch, err := clients.CoreV1().Services("").Watch(emptyOptions)
	if err != nil {
		return err
	}
	defer serviceWatch.Stop()

	endpointWatch, err := clients.CoreV1().Endpoints("").Watch(emptyOptions)
	if err != nil {
		return err
	}
	defer endpointWatch.Stop()

	var event watch.Event
	for {
		select {
		case event = <-ingressWatch.ResultChan():
		case event = <-serviceWatch.ResultChan():
		case event = <-endpointWatch.ResultChan():
			// ignore endpoint events selected namespaces, because noise. See e.g.:
			// https://github.com/kubernetes/kubernetes/issues/34627
			ev, ok := event.Object.(*v13.Endpoints)
			if ok {
				if _, ok := config.IgnoreNamespaces[ev.Namespace]; ok {
					log.Debug("Ignored kubernetes endpoint-API event",
						zap.String("type", string(event.Type)),
						zap.String("namespace", ev.Namespace),
						zap.String("name", ev.Name),
					)
					continue
				}
			}
		}
		log.Debug("Received kubernetes API event (backends)",
			zap.String("type", string(event.Type)),
			zap.String("object", fmt.Sprint(event.Object)),
		)
		updateChan <- util.NewReload("api-change-backends")
	}
}

func WatchSecrets(config *util.Config, updateChan chan util.Reload) error {
	clients, err := GetKubeClient(config.Kubeconfig)
	if err != nil {
		return err
	}

	secretWatch, err := clients.CoreV1().Secrets(config.CertNamespace).Watch(v1.ListOptions{
		LabelSelector: SECRET_HOSTNAME_LABEL,
	})
	if err != nil {
		return err
	}
	defer secretWatch.Stop()

	var event watch.Event
	for {
		event = <-secretWatch.ResultChan()
		log.Debug("Received kubernetes API event (secrets)",
			zap.String("type", string(event.Type)),
			zap.String("object", fmt.Sprint(event.Object)),
		)
		updateChan <- util.NewReload("api-change-secrets")
	}
}
