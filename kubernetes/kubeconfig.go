package kubernetes

import (
	"github.com/dbcdk/shelob/logging"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
)

var (
	log = logging.GetInstance()
)

func GetKubeConfig(kubeconfigFlag *string) (*rest.Config, error) {
	var kubeconfig string

	if kubeconfig = os.Getenv("KUBECONFIG"); kubeconfig != "" {
		log.Debug("Reading kubeconfig from environment", zap.String("path", kubeconfig))
	} else if kubeconfigFlag != nil {
		kubeconfig = *kubeconfigFlag
		log.Debug("Reading kubeconfig from flag", zap.String("path", kubeconfig))
	} else {
		log.Debug("No kubeconfig given, assuming in-cluster config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}
	return config, nil
}

func GetKubeClient(config *rest.Config) (*kubernetes.Clientset, error) {
	clients, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clients, nil
}

func GetInformerFactory(config *rest.Config, namespace string) (informers.SharedInformerFactory, error) {
	clients, err := GetKubeClient(config)
	if err != nil {
		return nil, err
	}
	return informers.NewSharedInformerFactoryWithOptions(clients, 0, informers.WithNamespace(namespace)), nil
}
