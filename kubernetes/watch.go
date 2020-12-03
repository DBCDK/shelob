package kubernetes

import (
	"fmt"

	"github.com/dbcdk/shelob/util"
	"go.uber.org/zap"
	apicorev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

func WatchBackends(config *util.Config, updateChan chan util.Reload) error {

	addRemoveFunc := func(obj interface{}) {
		log.Debug("Received kubernetes API event (backends)",
			zap.String("object", fmt.Sprint(obj)),
		)
		updateChan <- util.NewReload("api-change-backends")
	}
	updateFunc := func(oldObj interface{}, newObj interface{}) {
		addRemoveFunc(newObj)
	}
	endpointAddRemoveFunc := func(obj interface{}) {
		ev, ok := obj.(*apicorev1.Endpoints)
		if ok {
			if _, ok := config.IgnoreNamespaces[ev.Namespace]; ok {
				log.Debug("Ignored kubernetes endpoint-API event",
					zap.String("namespace", ev.Namespace),
					zap.String("name", ev.Name),
				)
				return
			}
			addRemoveFunc(obj)
		}
	}
	endpointUpdateFunc := func(oldObj interface{}, newObj interface{}) {
		endpointAddRemoveFunc(newObj)
	}

	stopChan := make(chan struct{})
	informerFactory, err := GetInformerFactory(config.Kubeconfig, apicorev1.NamespaceAll)
	if err != nil {
		return err
	}

	ingressv1Informer := informerFactory.Networking().V1().Ingresses().Informer()
	ingressv1Informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    addRemoveFunc,
		UpdateFunc: updateFunc,
		DeleteFunc: addRemoveFunc,
	})
	go ingressv1Informer.Run(stopChan)

	//TODO: Remove this when we're done supporting legacy apiversions
	ingressv1beta1Informer := informerFactory.Extensions().V1beta1().Ingresses().Informer()
	ingressv1beta1Informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    addRemoveFunc,
		UpdateFunc: updateFunc,
		DeleteFunc: addRemoveFunc,
	})
	go ingressv1beta1Informer.Run(stopChan)

	serviceInformer := informerFactory.Core().V1().Services().Informer()
	serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    addRemoveFunc,
		UpdateFunc: updateFunc,
		DeleteFunc: addRemoveFunc,
	})
	go serviceInformer.Run(stopChan)

	endpointInformer := informerFactory.Core().V1().Endpoints().Informer()
	endpointInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    endpointAddRemoveFunc,
		UpdateFunc: endpointUpdateFunc,
		DeleteFunc: endpointAddRemoveFunc,
	})
	go endpointInformer.Run(stopChan)

	<-config.State.ShutdownChan
	stopChan <- struct{}{}

	return nil
}

func WatchSecrets(config *util.Config, updateChan chan util.Reload) error {

	addRemoveFunc := func(obj interface{}) {
		log.Debug("Received kubernetes API event (secrets)",
			zap.String("object", fmt.Sprint(obj)),
		)

		if secret, ok := obj.(*apicorev1.Secret); ok {
			if _, hasLabel := secret.GetLabels()[SECRET_HOSTNAME_LABEL]; hasLabel {
				updateChan <- util.NewReload("api-change-secrets")
			}
		}
	}
	updateFunc := func(oldObj interface{}, newObj interface{}) {
		addRemoveFunc(newObj)
	}
	stopChan := make(chan struct{})

	informerFactory, err := GetInformerFactory(config.Kubeconfig, config.CertNamespace)
	if err != nil {
		return err
	}

	informer := informerFactory.Core().V1().Secrets().Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    addRemoveFunc,
		UpdateFunc: updateFunc,
		DeleteFunc: addRemoveFunc,
	})
	go informer.Run(stopChan)

	<-config.State.ShutdownChan
	stopChan <- struct{}{}

	return nil
}
