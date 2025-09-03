package jobs

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
)

// mockLogger implements the minimal Printf interface for testing
// type mockLogger struct{}

// func (l *mockLogger) Printf(format string, v ...interface{}) {}

// Test successful job execution and file writing
func TestJobCollect_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dc := &data_collector.DataCollector{
		BaseDir: tmpDir,
		Logger:  log.New(io.Discard, "", 0),
	}

	job := Job{
		Name:    "test-job",
		Timeout: time.Second,
		Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
			files := map[string][]byte{
				filepath.Join(dc.BaseDir, "output.txt"): []byte("hello world"),
			}
			ch <- JobResult{Files: files}
		},
	}

	err, skipped, _ := job.Collect(dc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if skipped {
		t.Fatalf("expected not skipped")
	}
	// Check file was written
	content, err := os.ReadFile(filepath.Join(tmpDir, "output.txt"))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(content) != "hello world" {
		t.Fatalf("unexpected file content: %s", string(content))
	}
}

// Test job skipped scenario
func TestJobCollect_Skipped(t *testing.T) {
	dc := &data_collector.DataCollector{
		BaseDir: t.TempDir(),
		Logger:  log.New(io.Discard, "", 0),
	}
	job := Job{
		Name:    "skip-job",
		Timeout: time.Second,
		Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
			ch <- JobResult{Skipped: true}
		},
	}
	err, skipped, _ := job.Collect(dc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !skipped {
		t.Fatalf("expected skipped")
	}
}

// Test job error scenario
func TestJobCollect_Error(t *testing.T) {
	dc := &data_collector.DataCollector{
		BaseDir: t.TempDir(),
		Logger:  log.New(io.Discard, "", 0),
	}
	job := Job{
		Name:    "error-job",
		Timeout: time.Second,
		Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
			ch <- JobResult{Error: errors.New("fail")}
		},
	}
	err, skipped, _ := job.Collect(dc)
	if err == nil || err.Error() != "fail" {
		t.Fatalf("expected error 'fail', got %v", err)
	}
	if skipped {
		t.Fatalf("expected not skipped")
	}
}

// Test job timeout scenario
func TestJobCollect_Timeout(t *testing.T) {
	dc := &data_collector.DataCollector{
		BaseDir: t.TempDir(),
		Logger:  log.New(io.Discard, "", 0),
	}
	job := Job{
		Name:    "timeout-job",
		Timeout: time.Millisecond * 10,
		Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
			time.Sleep(time.Second)
			ch <- JobResult{}
		},
	}
	err, skipped, _ := job.Collect(dc)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if skipped {
		t.Fatalf("expected not skipped")
	}
}
