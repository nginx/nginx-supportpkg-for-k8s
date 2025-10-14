package jobs

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNGXJobList_ExecNginxT(t *testing.T) {
	dc := mock.SetupMockDataCollector(t)

	// Create a fake pod named "nginx-123" in the "default" namespace
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-123",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx"},
			},
		},
	}
	dc.K8sCoreClientSet = fake.NewSimpleClientset(pod)

	// Mock PodExecutor
	dc.PodExecutor = func(namespace, pod, container string, command []string, ctx context.Context) ([]byte, error) {
		return []byte("nginx -T output"), nil
	}

	jobList := NGXJobList()
	if len(jobList) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobList))
	}
	job := jobList[0]
	ch := make(chan JobResult, 1)
	go job.Execute(dc, context.Background(), ch)
	select {
	case result := <-ch:
		if result.Error != nil {
			t.Fatalf("unexpected error: %v", result.Error)
		}
		found := false
		for file, content := range result.Files {
			if !strings.HasSuffix(file, "__nginx-t.txt") {
				t.Errorf("unexpected file name: %s", file)
			}
			if string(content) != "nginx -T output" {
				t.Errorf("unexpected file content: %s", string(content))
			}
			if !strings.HasPrefix(filepath.ToSlash(file), filepath.ToSlash(dc.BaseDir)) {
				t.Errorf("file path %s does not start with tmpDir %s", file, dc.BaseDir)
			}
			found = true
		}
		if !found {
			t.Errorf("no output file created by job")
		}
	case <-time.After(time.Second):
		t.Fatal("job execution timed out")
	}
}
