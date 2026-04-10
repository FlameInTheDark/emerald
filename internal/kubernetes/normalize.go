package kubernetes

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	// SourceTypeKubeconfig stores a user-supplied kubeconfig.
	SourceTypeKubeconfig = "kubeconfig"
	// SourceTypeManual stores a manually entered connection normalized as kubeconfig.
	SourceTypeManual = "manual"
)

// NormalizeClusterInput validates a settings payload and converts it to the
// stored normalized representation.
func NormalizeClusterInput(input ClusterInput) (*NormalizedCluster, error) {
	sourceType := strings.TrimSpace(input.SourceType)
	if sourceType == "" {
		sourceType = SourceTypeKubeconfig
	}

	switch sourceType {
	case SourceTypeKubeconfig:
		return normalizeKubeconfigInput(input)
	case SourceTypeManual:
		return normalizeManualInput(input)
	default:
		return nil, fmt.Errorf("unsupported source type %q", sourceType)
	}
}

// RecoverManualConfig reconstructs manual fields from a normalized kubeconfig.
func RecoverManualConfig(kubeconfig string, contextName string) (*ManualAuthConfig, string, error) {
	rawConfig, effectiveContext, err := loadRawConfig(kubeconfig, contextName)
	if err != nil {
		return nil, "", err
	}

	contextEntry, ok := rawConfig.Contexts[effectiveContext]
	if !ok {
		return nil, "", fmt.Errorf("context %q not found", effectiveContext)
	}

	clusterEntry, ok := rawConfig.Clusters[contextEntry.Cluster]
	if !ok {
		return nil, "", fmt.Errorf("cluster %q not found", contextEntry.Cluster)
	}

	authInfo, ok := rawConfig.AuthInfos[contextEntry.AuthInfo]
	if !ok {
		return nil, "", fmt.Errorf("auth info %q not found", contextEntry.AuthInfo)
	}

	manual := &ManualAuthConfig{
		Server:                clusterEntry.Server,
		Token:                 authInfo.Token,
		Username:              authInfo.Username,
		Password:              authInfo.Password,
		CAData:                string(clusterEntry.CertificateAuthorityData),
		ClientCertificateData: string(authInfo.ClientCertificateData),
		ClientKeyData:         string(authInfo.ClientKeyData),
		InsecureSkipTLSVerify: clusterEntry.InsecureSkipTLSVerify,
	}

	return manual, strings.TrimSpace(contextEntry.Namespace), nil
}

// ListContexts returns the names of contexts available in a kubeconfig.
func ListContexts(kubeconfig string) ([]string, error) {
	rawConfig, _, err := loadRawConfig(kubeconfig, "")
	if err != nil {
		return nil, err
	}

	contexts := make([]string, 0, len(rawConfig.Contexts))
	for name := range rawConfig.Contexts {
		contexts = append(contexts, name)
	}
	sort.Strings(contexts)
	return contexts, nil
}

// TestConnection validates auth and returns the effective session metadata.
func TestConnection(ctx context.Context, input ClusterInput) (*TestConnectionResult, error) {
	normalized, err := NormalizeClusterInput(input)
	if err != nil {
		return nil, err
	}

	session, err := NewSessionFromKubeconfig(normalized.Kubeconfig, normalized.ContextName)
	if err != nil {
		return nil, err
	}

	contexts, err := ListContexts(normalized.Kubeconfig)
	if err != nil {
		return nil, err
	}

	serverVersion, err := session.Clientset().Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("fetch server version: %w", err)
	}

	namespace := strings.TrimSpace(normalized.DefaultNamespace)
	if namespace == "" {
		namespace = session.Namespace()
	}
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}

	if _, err := session.Clientset().CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{}); err != nil {
		return nil, fmt.Errorf("validate namespace %q: %w", namespace, err)
	}

	return &TestConnectionResult{
		Contexts:         contexts,
		EffectiveContext: normalized.ContextName,
		DefaultNamespace: namespace,
		Server:           normalized.Server,
		ServerVersion:    serverVersion.GitVersion,
	}, nil
}

func normalizeKubeconfigInput(input ClusterInput) (*NormalizedCluster, error) {
	kubeconfig := strings.TrimSpace(input.Kubeconfig)
	if kubeconfig == "" {
		return nil, fmt.Errorf("kubeconfig is required")
	}

	rawConfig, effectiveContext, err := loadRawConfig(kubeconfig, input.ContextName)
	if err != nil {
		return nil, err
	}

	contextEntry := rawConfig.Contexts[effectiveContext]
	clusterEntry := rawConfig.Clusters[contextEntry.Cluster]

	defaultNamespace := strings.TrimSpace(input.DefaultNamespace)
	if defaultNamespace == "" {
		defaultNamespace = strings.TrimSpace(contextEntry.Namespace)
	}
	if defaultNamespace == "" {
		defaultNamespace = corev1.NamespaceDefault
	}

	return &NormalizedCluster{
		SourceType:       SourceTypeKubeconfig,
		Kubeconfig:       kubeconfig,
		ContextName:      effectiveContext,
		DefaultNamespace: defaultNamespace,
		Server:           strings.TrimSpace(clusterEntry.Server),
	}, nil
}

func normalizeManualInput(input ClusterInput) (*NormalizedCluster, error) {
	if input.Manual == nil {
		return nil, fmt.Errorf("manual configuration is required")
	}

	server := strings.TrimSpace(input.Manual.Server)
	if server == "" {
		return nil, fmt.Errorf("manual.server is required")
	}
	if strings.TrimSpace(input.Manual.Token) == "" &&
		(strings.TrimSpace(input.Manual.Username) == "" || strings.TrimSpace(input.Manual.Password) == "") &&
		strings.TrimSpace(input.Manual.ClientCertificateData) == "" {
		return nil, fmt.Errorf("manual auth requires a token, username/password, or client certificate")
	}

	contextName := strings.TrimSpace(input.ContextName)
	if contextName == "" {
		contextName = "emerald"
	}

	defaultNamespace := strings.TrimSpace(input.DefaultNamespace)
	if defaultNamespace == "" {
		defaultNamespace = corev1.NamespaceDefault
	}

	clusterName := "emerald-cluster"
	authName := "emerald-user"
	kubeconfigStruct := clientcmdapi.Config{
		APIVersion:     "v1",
		Kind:           "Config",
		CurrentContext: contextName,
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   server,
				InsecureSkipTLSVerify:    input.Manual.InsecureSkipTLSVerify,
				CertificateAuthorityData: decodeMaybeBase64(input.Manual.CAData),
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			authName: {
				Token:                 strings.TrimSpace(input.Manual.Token),
				Username:              strings.TrimSpace(input.Manual.Username),
				Password:              strings.TrimSpace(input.Manual.Password),
				ClientCertificateData: decodeMaybeBase64(input.Manual.ClientCertificateData),
				ClientKeyData:         decodeMaybeBase64(input.Manual.ClientKeyData),
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:   clusterName,
				AuthInfo:  authName,
				Namespace: defaultNamespace,
			},
		},
	}

	kubeconfigBytes, err := clientcmd.Write(kubeconfigStruct)
	if err != nil {
		return nil, fmt.Errorf("serialize kubeconfig: %w", err)
	}

	return &NormalizedCluster{
		SourceType:       SourceTypeManual,
		Kubeconfig:       string(kubeconfigBytes),
		ContextName:      contextName,
		DefaultNamespace: defaultNamespace,
		Server:           server,
	}, nil
}

func loadRawConfig(kubeconfig string, overrideContext string) (clientcmdapi.Config, string, error) {
	rawConfig, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		return clientcmdapi.Config{}, "", fmt.Errorf("parse kubeconfig: %w", err)
	}

	effectiveContext := strings.TrimSpace(overrideContext)
	if effectiveContext == "" {
		effectiveContext = strings.TrimSpace(rawConfig.CurrentContext)
	}
	if effectiveContext == "" {
		contexts := make([]string, 0, len(rawConfig.Contexts))
		for name := range rawConfig.Contexts {
			contexts = append(contexts, name)
		}
		sort.Strings(contexts)
		if len(contexts) == 1 {
			effectiveContext = contexts[0]
		}
	}
	if effectiveContext == "" {
		return clientcmdapi.Config{}, "", fmt.Errorf("kubeconfig does not define an active context")
	}
	if _, ok := rawConfig.Contexts[effectiveContext]; !ok {
		return clientcmdapi.Config{}, "", fmt.Errorf("context %q not found", effectiveContext)
	}

	return *rawConfig, effectiveContext, nil
}

func decodeMaybeBase64(value string) []byte {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if strings.Contains(trimmed, "-----BEGIN") {
		return []byte(trimmed)
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err == nil {
		return decoded
	}
	decoded, err = base64.RawStdEncoding.DecodeString(trimmed)
	if err == nil {
		return decoded
	}
	return []byte(trimmed)
}
