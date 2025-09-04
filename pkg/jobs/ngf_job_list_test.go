package jobs

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNGFJobList(t *testing.T) {
	jobs := NGFJobList()
	if len(jobs) == 0 {
		t.Error("expected jobs to be returned")
	}

	expectedJobs := []string{"exec-nginx-gateway-version", "exec-nginx-t", "crd-objects"}
	if len(jobs) != len(expectedJobs) {
		t.Errorf("expected %d jobs, got %d", len(expectedJobs), len(jobs))
	}

	for i, job := range jobs {
		if job.Name != expectedJobs[i] {
			t.Errorf("expected job name %s, got %s", expectedJobs[i], job.Name)
		}
		if job.Execute == nil {
			t.Errorf("job %s should have Execute function", job.Name)
		}
		if job.Timeout == 0 {
			t.Errorf("job %s should have timeout set", job.Name)
		}
	}
}

func TestNGFJobExecNginxGatewayVersion(t *testing.T) {
	tmpDir := t.TempDir()
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-gateway-test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx-gateway"},
			},
		},
	})

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"default"},
		K8sCoreClientSet: client,
		Logger:           log.New(io.Discard, "", 0),
		PodExecutor: func(namespace, pod, container string, command []string, ctx context.Context) ([]byte, error) {
			return []byte("gateway version output"), nil
		},
	}

	jobs := NGFJobList()
	var versionJob Job
	for _, job := range jobs {
		if job.Name == "exec-nginx-gateway-version" {
			versionJob = job
			break
		}
	}

	ch := make(chan JobResult, 1)
	ctx := context.Background()

	versionJob.Execute(dc, ctx, ch)
	result := <-ch

	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}

	if len(result.Files) == 0 {
		t.Error("expected files to be created")
	}

	found := false
	for filename := range result.Files {
		if strings.Contains(filename, "nginx-gateway-version.txt") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected nginx-gateway-version.txt file to be created")
	}
}

func TestNGFJobExecNginxT(t *testing.T) {
	tmpDir := t.TempDir()
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-gateway-test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx"},
			},
		},
	})

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"default"},
		K8sCoreClientSet: client,
		Logger:           log.New(io.Discard, "", 0),
		PodExecutor: func(namespace, pod, container string, command []string, ctx context.Context) ([]byte, error) {
			return []byte("nginx -t output"), nil
		},
	}

	jobs := NGFJobList()
	var tJob Job
	for _, job := range jobs {
		if job.Name == "exec-nginx-t" {
			tJob = job
			break
		}
	}

	ch := make(chan JobResult, 1)
	ctx := context.Background()

	tJob.Execute(dc, ctx, ch)
	result := <-ch

	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}

	if len(result.Files) == 0 {
		t.Error("expected files to be created")
	}

	found := false
	for filename := range result.Files {
		if strings.Contains(filename, "nginx-t.txt") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected nginx-t.txt file to be created")
	}
}
