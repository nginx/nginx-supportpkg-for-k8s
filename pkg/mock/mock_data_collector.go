package mock

import (
	"io"
	"log"
	"testing"

	helmclient "github.com/mittwald/go-helm-client"
	mockHelmClient "github.com/mittwald/go-helm-client/mock"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	"go.uber.org/mock/gomock"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	"k8s.io/utils/ptr"
)

func SetupMockDataCollector(t *testing.T) *data_collector.DataCollector {
	t.Helper()

	tmpDir := t.TempDir()

	// Seed fake objects (namespace default implied by metadata)
	objs := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c1", Image: "nginx:latest"}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-1", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "demo"}},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dep-1", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(1)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "demo"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "dep-c1", Image: "nginx:latest"}},
					},
				},
			},
		},
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "role-1", Namespace: "default"},
			Rules:      []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cm-1", Namespace: "default"},
			Data:       map[string]string{"k": "v"},
		},
	}

	client := fake.NewSimpleClientset(objs...)
	// Mock rest.Config
	restConfig := &rest.Config{
		Host: "https://mock-k8s-server",
	}

	// Create a CRD clientset (using the real clientset, but not actually connecting)
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testcrd.example.com",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:     "TestCRD",
				Plural:   "testcrds",
				Singular: "testcrd",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
				},
			},
		},
	}

	crdClient := apiextensionsfake.NewSimpleClientset(crd)
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
