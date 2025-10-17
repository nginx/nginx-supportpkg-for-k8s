package mock

import (
	"bytes"
	"embed"
	"io"
	"log"
	"testing"

	helmclient "github.com/mittwald/go-helm-client"
	mockHelmClient "github.com/mittwald/go-helm-client/mock"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	"go.uber.org/mock/gomock"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	"sigs.k8s.io/yaml"
)

//go:embed testdata/crds.yaml
//go:embed testdata/objects.yaml
var testDataFS embed.FS

func loadObjectsFromYAML(filename string) ([]runtime.Object, error) {
	data, err := testDataFS.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var objects []runtime.Object

	// Split YAML documents by "---"
	docs := bytes.Split(data, []byte("---"))

	for _, doc := range docs {
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}

		// Use the universal deserializer directly
		obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(doc, nil, nil)
		if err != nil {
			return nil, err
		}

		if obj != nil {
			objects = append(objects, obj)
		}
	}
	return objects, nil
}

func SetupMockDataCollector(t *testing.T) *data_collector.DataCollector {
	t.Helper()

	tmpDir := t.TempDir()

	// Load objects from YAML instead of hardcoding
	objs, err := loadObjectsFromYAML("testdata/objects.yaml")
	if err != nil {
		t.Fatalf("Failed to load test objects: %v", err)
	}

	client := fake.NewSimpleClientset(objs...)

	// Mock rest.Config
	restConfig := &rest.Config{
		Host: "https://mock-k8s-server",
	}

	// Use embedded file
	data, err := testDataFS.ReadFile("testdata/crds.yaml")
	if err != nil {
		t.Fatalf("failed to read embedded testdata/crds.yaml: %v", err)
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(data, crd); err != nil {
		t.Fatalf("failed to unmarshal CRD from testdata/crds.yaml: %v", err)
	}

	crdClient := apiextensionsfake.NewClientset(crd)
	metricsClient := metricsfake.NewSimpleClientset()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	helmClient := mockHelmClient.NewMockClient(ctrl)

	helmClient.EXPECT().GetSettings().Return(&cli.EnvSettings{}).AnyTimes()
	var mockedRelease = release.Release{
		Name:      "test",
		Namespace: "test",
		Manifest:  "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: example-config\n  namespace: default\ndata:\n  key: value\n",
	}
	helmClient.EXPECT().ListDeployedReleases().Return([]*release.Release{&mockedRelease}, nil).AnyTimes()

	return &data_collector.DataCollector{
		BaseDir:             tmpDir,
		Namespaces:          []string{"default"},
		Logger:              log.New(io.Discard, "", 0),
		K8sCoreClientSet:    client,
		K8sCrdClientSet:     crdClient,
		K8sRestConfig:       restConfig,
		K8sMetricsClientSet: metricsClient,
		K8sHelmClientSet:    map[string]helmclient.Client{"default": helmClient},
	}
}
