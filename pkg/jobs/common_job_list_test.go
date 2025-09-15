package jobs

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/mock"
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
