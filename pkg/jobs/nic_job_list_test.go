package jobs

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/crds"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// mockPodExecutor simulates PodExecutor for testing
func mockPodExecutor(namespace, pod, container string, command []string, ctx context.Context) ([]byte, error) {
	return []byte("mock-output"), nil
}

// mockQueryCRD simulates QueryCRD for testing
func mockQueryCRD(crd crds.Crd, namespace string, ctx context.Context) ([]byte, error) {
	return json.Marshal(map[string]string{"kind": crd.Resource})
}

func TestNICJobList_ExecJobs(t *testing.T) {
	dc := mock.SetupMockDataCollector(t)
	dc.Namespaces = []string{"test-ns"}

	// Mock PodExecutor and QueryCRD
	dc.PodExecutor = mockPodExecutor
	dc.QueryCRD = mockQueryCRD

	// Use a real or fake clientset (kubernetes.Interface)
	dc.K8sCoreClientSet = fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "ingress-pod", Namespace: "test-ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx-ingress", Image: "nginx-ingress:latest"},
			},
		},
	})

	jobList := NICJobList()
	for _, job := range jobList {
		ch := make(chan JobResult, 1)
		go job.Execute(dc, context.Background(), ch)
		select {
		case result := <-ch:
			if result.Error != nil {
				t.Errorf("Job %s returned error: %v", job.Name, result.Error)
			}
			for file, content := range result.Files {
				if !strings.HasPrefix(filepath.ToSlash(file), filepath.ToSlash(dc.BaseDir)) {
					t.Errorf("File path %s does not start with tmpDir", file)
				}
				if len(content) == 0 {
					t.Errorf("File %s has empty content", file)
				}
			}
		case <-time.After(time.Second):
			t.Errorf("Job %s timed out", job.Name)
		}
	}
}

func TestNICJobList_CRDObjects(t *testing.T) {
	dc := mock.SetupMockDataCollector(t)
	dc.Namespaces = []string{"test-ns"}
	dc.QueryCRD = mockQueryCRD

	jobList := NICJobList()
	var found bool
	for _, job := range jobList {
		if job.Name == "crd-objects" {
			found = true
			ch := make(chan JobResult, 1)
			go job.Execute(dc, context.Background(), ch)
			select {
			case result := <-ch:
				if result.Error != nil {
					t.Errorf("CRD job returned error: %v", result.Error)
				}
				for file, content := range result.Files {
					if !strings.HasPrefix(filepath.ToSlash(file), filepath.ToSlash(dc.BaseDir)) {
						t.Errorf("File path %s does not start with tmpDir", file)
					}
					var out map[string]interface{}
					if err := json.Unmarshal(content, &out); err != nil {
						t.Errorf("Invalid JSON in file %s: %v", file, err)
					}
				}
			case <-time.After(time.Second):
				t.Errorf("CRD job timed out")
			}
		}
	}
	if !found {
		t.Errorf("crd-objects job not found in NICJobList")
	}
}
