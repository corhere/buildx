package context

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/docker/cli/cli/context"
	"github.com/docker/cli/cli/context/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func testEndpoint(server, defaultNamespace string, ca, cert, key []byte, skipTLSVerify bool) Endpoint {
	var tlsData *context.TLSData
	if ca != nil || cert != nil || key != nil {
		tlsData = &context.TLSData{
			CA:   ca,
			Cert: cert,
			Key:  key,
		}
	}
	return Endpoint{
		EndpointMeta: EndpointMeta{
			EndpointMetaBase: context.EndpointMetaBase{
				Host:          server,
				SkipTLSVerify: skipTLSVerify,
			},
			DefaultNamespace: defaultNamespace,
		},
		TLSData: tlsData,
	}
}

var testStoreCfg = store.NewConfig(
	func() interface{} {
		return &map[string]interface{}{}
	},
	store.EndpointTypeGetter(KubernetesEndpoint, func() interface{} { return &EndpointMeta{} }),
)

func TestSaveLoadContexts(t *testing.T) {
	storeDir, err := ioutil.TempDir("", "test-load-save-k8-context")
	require.NoError(t, err)
	defer os.RemoveAll(storeDir)
	store := store.New(storeDir, testStoreCfg)
	require.NoError(t, save(store, testEndpoint("https://test", "test", nil, nil, nil, false), "raw-notls"))
	require.NoError(t, save(store, testEndpoint("https://test", "test", nil, nil, nil, true), "raw-notls-skip"))
	require.NoError(t, save(store, testEndpoint("https://test", "test", []byte("ca"), []byte("cert"), []byte("key"), true), "raw-tls"))

	kcFile, err := ioutil.TempFile(os.TempDir(), "test-load-save-k8-context")
	require.NoError(t, err)
	defer os.Remove(kcFile.Name())
	defer kcFile.Close()
	cfg := clientcmdapi.NewConfig()
	cfg.AuthInfos["user"] = clientcmdapi.NewAuthInfo()
	cfg.Contexts["context1"] = clientcmdapi.NewContext()
	cfg.Clusters["cluster1"] = clientcmdapi.NewCluster()
	cfg.Contexts["context2"] = clientcmdapi.NewContext()
	cfg.Clusters["cluster2"] = clientcmdapi.NewCluster()
	cfg.AuthInfos["user"].ClientCertificateData = []byte("cert")
	cfg.AuthInfos["user"].ClientKeyData = []byte("key")
	cfg.Clusters["cluster1"].Server = "https://server1"
	cfg.Clusters["cluster1"].InsecureSkipTLSVerify = true
	cfg.Clusters["cluster2"].Server = "https://server2"
	cfg.Clusters["cluster2"].CertificateAuthorityData = []byte("ca")
	cfg.Contexts["context1"].AuthInfo = "user"
	cfg.Contexts["context1"].Cluster = "cluster1"
	cfg.Contexts["context1"].Namespace = "namespace1"
	cfg.Contexts["context2"].AuthInfo = "user"
	cfg.Contexts["context2"].Cluster = "cluster2"
	cfg.Contexts["context2"].Namespace = "namespace2"
	cfg.CurrentContext = "context1"
	cfgData, err := clientcmd.Write(*cfg)
	require.NoError(t, err)
	_, err = kcFile.Write(cfgData)
	require.NoError(t, err)
	kcFile.Close()

	epDefault, err := FromKubeConfig(kcFile.Name(), "", "")
	require.NoError(t, err)
	epContext2, err := FromKubeConfig(kcFile.Name(), "context2", "namespace-override")
	require.NoError(t, err)
	require.NoError(t, save(store, epDefault, "embed-default-context"))
	require.NoError(t, save(store, epContext2, "embed-context2"))

	rawNoTLSMeta, err := store.GetMetadata("raw-notls")
	require.NoError(t, err)
	rawNoTLSSkipMeta, err := store.GetMetadata("raw-notls-skip")
	require.NoError(t, err)
	rawTLSMeta, err := store.GetMetadata("raw-tls")
	require.NoError(t, err)
	embededDefaultMeta, err := store.GetMetadata("embed-default-context")
	require.NoError(t, err)
	embededContext2Meta, err := store.GetMetadata("embed-context2")
	require.NoError(t, err)

	rawNoTLS := EndpointFromContext(rawNoTLSMeta)
	rawNoTLSSkip := EndpointFromContext(rawNoTLSSkipMeta)
	rawTLS := EndpointFromContext(rawTLSMeta)
	embededDefault := EndpointFromContext(embededDefaultMeta)
	embededContext2 := EndpointFromContext(embededContext2Meta)

	rawNoTLSEP, err := rawNoTLS.WithTLSData(store, "raw-notls")
	require.NoError(t, err)
	checkClientConfig(t, rawNoTLSEP, "https://test", "test", nil, nil, nil, false)
	rawNoTLSSkipEP, err := rawNoTLSSkip.WithTLSData(store, "raw-notls-skip")
	require.NoError(t, err)
	checkClientConfig(t, rawNoTLSSkipEP, "https://test", "test", nil, nil, nil, true)
	rawTLSEP, err := rawTLS.WithTLSData(store, "raw-tls")
	require.NoError(t, err)
	checkClientConfig(t, rawTLSEP, "https://test", "test", []byte("ca"), []byte("cert"), []byte("key"), true)
	embededDefaultEP, err := embededDefault.WithTLSData(store, "embed-default-context")
	require.NoError(t, err)
	checkClientConfig(t, embededDefaultEP, "https://server1", "namespace1", nil, []byte("cert"), []byte("key"), true)
	embededContext2EP, err := embededContext2.WithTLSData(store, "embed-context2")
	require.NoError(t, err)
	checkClientConfig(t, embededContext2EP, "https://server2", "namespace-override", []byte("ca"), []byte("cert"), []byte("key"), false)
}

func checkClientConfig(t *testing.T, ep Endpoint, server, namespace string, ca, cert, key []byte, skipTLSVerify bool) {
	config := ep.KubernetesConfig()
	cfg, err := config.ClientConfig()
	require.NoError(t, err)
	ns, _, _ := config.Namespace()
	assert.Equal(t, server, cfg.Host)
	assert.Equal(t, namespace, ns)
	assert.Equal(t, ca, cfg.CAData)
	assert.Equal(t, cert, cfg.CertData)
	assert.Equal(t, key, cfg.KeyData)
	assert.Equal(t, skipTLSVerify, cfg.Insecure)
}

func save(s store.Writer, ep Endpoint, name string) error {
	meta := store.Metadata{
		Endpoints: map[string]interface{}{
			KubernetesEndpoint: ep.EndpointMeta,
		},
		Name: name,
	}
	if err := s.CreateOrUpdate(meta); err != nil {
		return err
	}
	return s.ResetEndpointTLSMaterial(name, KubernetesEndpoint, ep.TLSData.ToStoreTLSData())
}

func TestSaveLoadGKEConfig(t *testing.T) {
	storeDir, err := ioutil.TempDir("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(storeDir)
	store := store.New(storeDir, testStoreCfg)
	cfg, err := clientcmd.LoadFromFile("fixtures/gke-kubeconfig")
	require.NoError(t, err)
	clientCfg := clientcmd.NewDefaultClientConfig(*cfg, &clientcmd.ConfigOverrides{})
	expectedCfg, err := clientCfg.ClientConfig()
	require.NoError(t, err)
	ep, err := FromKubeConfig("fixtures/gke-kubeconfig", "", "")
	require.NoError(t, err)
	require.NoError(t, save(store, ep, "gke-context"))
	persistedMetadata, err := store.GetMetadata("gke-context")
	require.NoError(t, err)
	persistedEPMeta := EndpointFromContext(persistedMetadata)
	assert.True(t, persistedEPMeta != nil)
	persistedEP, err := persistedEPMeta.WithTLSData(store, "gke-context")
	require.NoError(t, err)
	persistedCfg := persistedEP.KubernetesConfig()
	actualCfg, err := persistedCfg.ClientConfig()
	require.NoError(t, err)
	assert.Equal(t, expectedCfg.AuthProvider, actualCfg.AuthProvider)
}

func TestSaveLoadEKSConfig(t *testing.T) {
	storeDir, err := ioutil.TempDir("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(storeDir)
	store := store.New(storeDir, testStoreCfg)
	cfg, err := clientcmd.LoadFromFile("fixtures/eks-kubeconfig")
	require.NoError(t, err)
	clientCfg := clientcmd.NewDefaultClientConfig(*cfg, &clientcmd.ConfigOverrides{})
	expectedCfg, err := clientCfg.ClientConfig()
	require.NoError(t, err)
	ep, err := FromKubeConfig("fixtures/eks-kubeconfig", "", "")
	require.NoError(t, err)
	require.NoError(t, save(store, ep, "eks-context"))
	persistedMetadata, err := store.GetMetadata("eks-context")
	require.NoError(t, err)
	persistedEPMeta := EndpointFromContext(persistedMetadata)
	assert.True(t, persistedEPMeta != nil)
	persistedEP, err := persistedEPMeta.WithTLSData(store, "eks-context")
	require.NoError(t, err)
	persistedCfg := persistedEP.KubernetesConfig()
	actualCfg, err := persistedCfg.ClientConfig()
	require.NoError(t, err)
	assert.Equal(t, expectedCfg.ExecProvider, actualCfg.ExecProvider)
}

func TestSaveLoadK3SConfig(t *testing.T) {
	storeDir, err := ioutil.TempDir("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(storeDir)
	store := store.New(storeDir, testStoreCfg)
	cfg, err := clientcmd.LoadFromFile("fixtures/k3s-kubeconfig")
	require.NoError(t, err)
	clientCfg := clientcmd.NewDefaultClientConfig(*cfg, &clientcmd.ConfigOverrides{})
	expectedCfg, err := clientCfg.ClientConfig()
	require.NoError(t, err)
	ep, err := FromKubeConfig("fixtures/k3s-kubeconfig", "", "")
	require.NoError(t, err)
	require.NoError(t, save(store, ep, "k3s-context"))
	persistedMetadata, err := store.GetMetadata("k3s-context")
	require.NoError(t, err)
	persistedEPMeta := EndpointFromContext(persistedMetadata)
	assert.True(t, persistedEPMeta != nil)
	persistedEP, err := persistedEPMeta.WithTLSData(store, "k3s-context")
	require.NoError(t, err)
	persistedCfg := persistedEP.KubernetesConfig()
	actualCfg, err := persistedCfg.ClientConfig()
	require.NoError(t, err)
	assert.True(t, len(actualCfg.Username) > 0)
	assert.True(t, len(actualCfg.Password) > 0)
	assert.Equal(t, expectedCfg.Username, actualCfg.Username)
	assert.Equal(t, expectedCfg.Password, actualCfg.Password)
}
