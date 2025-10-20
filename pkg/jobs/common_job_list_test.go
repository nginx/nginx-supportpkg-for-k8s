package jobs

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/mock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing" // Add this line
)

func TestCommonJobList_SelectedJobsProduceFiles(t *testing.T) {
	dc := mock.SetupMockDataCollector(t)
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
	dc := mock.SetupMockDataCollector(t)
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

func TestCommonJobList_PodListError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock clients
	mockClient := fake.NewSimpleClientset()

	// Add a reactor that returns an error for pod list operations
	mockClient.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("mock API error: pods not available")
	})

	// Setup data collector with error-prone client
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"default", "test-namespace"},
		Logger:           log.New(&logOutput, "", 0),
		K8sCoreClientSet: mockClient,
	}

	// Execute the pod-list job
	var podJob []Job
	jobs := CommonJobList()
	for _, job := range jobs {
		if job.Name == "pod-list" || job.Name == "collect-pods-logs" {
			podJob = append(podJob, job)
		}
	}
	// podJob := jobs[0] // First job is pod-list

	ctx := context.Background()
	ch := make(chan JobResult, 1)
	for _, job := range podJob {
		job.Execute(dc, ctx, ch)

		result := <-ch

		// Assertions
		logContent := logOutput.String()
		assert.Contains(t, logContent, "Could not retrieve pod list for namespace default")
		assert.Contains(t, logContent, "Could not retrieve pod list for namespace test-namespace")
		assert.Contains(t, logContent, "mock API error: pods not available")

		// No files should be created since API calls failed
		assert.Empty(t, result.Files)
		assert.Nil(t, result.Error) // The job itself doesn't fail, just logs errors
	}
}
