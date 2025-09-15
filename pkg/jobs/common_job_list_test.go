package jobs

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	helmclient "github.com/mittwald/go-helm-client"
	mockHelmClient "github.com/mittwald/go-helm-client/mock"
	"go.uber.org/mock/gomock"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

// helper creates int32 ptr
func i32(v int32) *int32 { return &v }

func setupDataCollector(t *testing.T) *data_collector.DataCollector {
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
				Replicas: i32(1),
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
	// crdClient := &apiextensionsclientset.Clientset{}
	crdClient := apiextensionsfake.NewSimpleClientset(crd)
	metricsClient := metricsfake.NewSimpleClientset()
	// helmClient := &FakeHelmClient{}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	helmClient := mockHelmClient.NewMockClient(ctrl)
	if helmClient == nil {
		t.Fail()
	}
	helmClient.EXPECT().GetSettings().Return(&cli.EnvSettings{}).AnyTimes()
	var mockedRelease = release.Release{Name: "test", Namespace: "test", Manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: example-config\n  namespace: default\ndata:\n  key: value\n"}
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

		// Leave other client sets nil; we will not execute jobs that depend on them in this focused test.
	}
}

func TestCommonJobList_SelectedJobsProduceFiles(t *testing.T) {
	dc := setupDataCollector(t)
	jobList := CommonJobList()

	for _, job := range jobList {

		ch := make(chan JobResult, 1)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		go job.Execute(dc, ctx, ch)

		select {
		case res := <-ch:
			if res.Error != nil {
				t.Fatalf("job %s returned unexpected error: %v", job.Name, res.Error)
			}
			if len(res.Files) == 0 {
				t.Fatalf("job %s produced no files", job.Name)
			}
			// Basic path sanity + non-empty content
			for path, content := range res.Files {
				if len(content) == 0 {
					t.Fatalf("job %s file %s has empty content", job.Name, path)
				}
				if !strings.HasPrefix(filepath.ToSlash(path), filepath.ToSlash(dc.BaseDir)) {
					t.Fatalf("job %s file path %s does not start with basedir %s", job.Name, path, dc.BaseDir)
				}
			}
		case <-ctx.Done():
			t.Fatalf("job %s timed out", job.Name)
		}
	}
}

func TestCommonJobList_PodListJSONKeyPresence(t *testing.T) {
	dc := setupDataCollector(t)
	var podListJob *Job
	jobs := CommonJobList()
	for i, j := range jobs {
		if j.Name == "pod-list" {
			podListJob = &jobs[i]
			break
		}
	}
	if podListJob == nil {
		t.Fatalf("pod-list job not found")
	}

	ch := make(chan JobResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go podListJob.Execute(dc, ctx, ch)

	res := <-ch
	if res.Error != nil {
		t.Fatalf("pod-list job returned error: %v", res.Error)
	}
	if len(res.Files) != 1 {
		t.Fatalf("expected 1 file from pod-list job, got %d", len(res.Files))
	}
	for path, content := range res.Files {
		if filepath.Base(path) != "pods.json" {
			t.Fatalf("expected pods.json file, got %s", path)
		}
		// Quick check JSON starts with '{' (marshaled list) or '[' depending on structure (PodList marshals to object)
		if len(content) == 0 || content[0] != '{' {
			t.Fatalf("unexpected JSON content in %s", path)
		}
	}
}
