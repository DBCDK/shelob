package kubernetes

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/dbcdk/shelob/util"
	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const SECRET_HOSTNAME_LABEL = "ingress.hostname"

func GetCerts(config *util.Config, namespace string) (map[string]*tls.Certificate, error) {

	clients, err := GetKubeClient(config.Kubeconfig)
	if err != nil {
		return nil, err
	}

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	secrets, err := clients.CoreV1().Secrets(namespace).List(ctx, v1.ListOptions{
		LabelSelector: SECRET_HOSTNAME_LABEL,
	})
	if err != nil {
		return nil, err
	}

	certs := make(map[string]*tls.Certificate)
	for _, s := range secrets.Items {
		cert, err := parsex509Secret(s.Data)
		hostName := s.Labels[SECRET_HOSTNAME_LABEL]
		if err != nil {
			log.Error("Failed to parse x509 keypair",
				zap.String("secretNamespace", s.Namespace),
				zap.String("secretName", s.Name),
				zap.String("hostname", hostName),
				zap.String("error", err.Error()),
			)
		}
		certs[hostName] = cert
	}

	return certs, nil
}

func parsex509Secret(data map[string][]byte) (*tls.Certificate, error) {
	cert, ok := data["cert"]
	if !ok {
		return nil, fmt.Errorf("Public key part ('cert') missing")
	}
	key, ok := data["key"]
	if !ok {
		return nil, fmt.Errorf("Private key part ('key') missing")
	}

	out, err := tls.X509KeyPair(cert, key)
	return &out, err
}
