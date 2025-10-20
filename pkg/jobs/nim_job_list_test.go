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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestNIMJobList_ExecJobs(t *testing.T) {
	dc := mock.SetupMockDataCollector(t)
	dc.Namespaces = []string{"default"}

	// Create fake pods for each job type
	pods := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "apigw-123",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "apigw"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "clickhouse-456",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "clickhouse-server"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "core-789",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "core"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dpm-101",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "dpm"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "integrations-102",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "integrations"}},
			},
		},
	}

	objects := make([]runtime.Object, len(pods))
	for i, pod := range pods {
		objects[i] = pod
	}
	dc.K8sCoreClientSet = fake.NewClientset(objects...)

	// Mock PodExecutor to return predictable output
	dc.PodExecutor = func(namespace, pod, container string, command []string, ctx context.Context) ([]byte, error) {
		return []byte(strings.Join(command, " ")), nil
	}

	// Run all jobs in NIMJobList
	for _, job := range NIMJobList() {
		ch := make(chan JobResult, 1)
		go job.Execute(dc, context.Background(), ch)
		select {
		case result := <-ch:
			if result.Error != nil {
				t.Errorf("Job %s returned error: %v", job.Name, result.Error)
			}
			for file, content := range result.Files {
				if !strings.HasPrefix(filepath.ToSlash(file), filepath.ToSlash(dc.BaseDir)) {
					t.Errorf("File path %s does not start with tmpDir %s", file, dc.BaseDir)
				}
				if len(content) == 0 {
					t.Errorf("File %s has empty content", file)
				}
			}
		case <-time.After(2 * time.Second):
			t.Errorf("Job %s timed out", job.Name)
		}
	}
}

func TestNIMJobList_ExcludeFlags(t *testing.T) {
	dc := mock.SetupMockDataCollector(t)
	dc.Namespaces = []string{"default"}
	dc.K8sCoreClientSet = fake.NewClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clickhouse-456",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "clickhouse-server"}},
		},
	})
	dc.PodExecutor = func(namespace, pod, container string, command []string, ctx context.Context) ([]byte, error) {
		return []byte("output"), nil
	}
	// Test ExcludeTimeSeriesData for exec-clickhouse-data
	dc.ExcludeTimeSeriesData = true
	for _, job := range NIMJobList() {
		if job.Name == "exec-clickhouse-data" {
			ch := make(chan JobResult, 1)
			go job.Execute(dc, context.Background(), ch)
			select {
			case result := <-ch:
				if !result.Skipped {
					t.Errorf("Expected job to be skipped when ExcludeTimeSeriesData is true")
				}
			case <-time.After(time.Second):
				t.Fatal("Job exec-clickhouse-data timed out")
			}
		}
	}

	// Test ExcludeDBData for exec-dqlite-dump
	dc.ExcludeDBData = true
	for _, job := range NIMJobList() {
		if job.Name == "exec-dqlite-dump" {
			ch := make(chan JobResult, 1)
			go job.Execute(dc, context.Background(), ch)
			select {
			case result := <-ch:
				if !result.Skipped {
					t.Errorf("Expected job to be skipped when ExcludeDBData is true")
				}
			case <-time.After(time.Second):
				t.Fatal("Job exec-dqlite-dump timed out")
			}
		}
	}
}

func TestNIMJobList_PodListErrors(t *testing.T) {
	tests := []struct {
		name     string
		jobName  string
		jobIndex int
	}{
		{
			name:     "exec-apigw-nginx-t pod list error",
			jobName:  "exec-apigw-nginx-t",
			jobIndex: 0,
		},
		{
			name:     "exec-apigw-nginx-version pod list error",
			jobName:  "exec-apigw-nginx-version",
			jobIndex: 1,
		},
		{
			name:     "exec-clickhouse-version pod list error",
			jobName:  "exec-clickhouse-version",
			jobIndex: 2,
		},
		{
			name:     "exec-clickhouse-data pod list error",
			jobName:  "exec-clickhouse-data",
			jobIndex: 3,
		},
		{
			name:     "exec-dqlite-dump pod list error",
			jobName:  "exec-dqlite-dump",
			jobIndex: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			// Create a fake client that will return an error for pod listing
			client := fake.NewClientset()
			client.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, fmt.Errorf("failed to retrieve pod list")
			})

			dc := &data_collector.DataCollector{
				BaseDir:               tmpDir,
				Namespaces:            []string{"default", "nim-system"},
				Logger:                log.New(&logOutput, "", 0),
				K8sCoreClientSet:      client,
				ExcludeTimeSeriesData: false,
				ExcludeDBData:         false,
				PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
					return []byte("mock output"), nil
				},
			}

			// Get the specific job
			jobs := NIMJobList()
			execJob := jobs[tt.jobIndex]
			assert.Equal(t, tt.jobName, execJob.Name, "Job name should match expected")

			// Execute the job
			ctx := context.Background()
			ch := make(chan JobResult, 1)
			execJob.Execute(dc, ctx, ch)

			result := <-ch
			logContent := logOutput.String()

			// Verify the error was logged for each namespace
			assert.Contains(t, logContent, "Could not retrieve pod list for namespace default: failed to retrieve pod list")
			assert.Contains(t, logContent, "Could not retrieve pod list for namespace nim-system: failed to retrieve pod list")

			// Verify no files were created since pod listing failed
			assert.Empty(t, result.Files, "No files should be created when pod list fails")
			assert.Nil(t, result.Error, "Job should not fail, just log the error")
		})
	}
}

func TestNIMJobList_MultipleNamespaces_PodListErrors(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	// Create a fake client that returns different errors for different namespaces
	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		listAction := action.(k8stesting.ListAction)
		namespace := listAction.GetNamespace()

		switch namespace {
		case "error-ns1":
			return true, nil, fmt.Errorf("network timeout")
		case "error-ns2":
			return true, nil, fmt.Errorf("permission denied")
		case "error-ns3":
			return true, nil, fmt.Errorf("resource not found")
		default:
			// Let other namespaces succeed (but with no nim pods)
			return false, nil, nil
		}
	})

	dc := &data_collector.DataCollector{
		BaseDir:               tmpDir,
		Namespaces:            []string{"error-ns1", "error-ns2", "error-ns3", "success-ns"},
		Logger:                log.New(&logOutput, "", 0),
		K8sCoreClientSet:      client,
		ExcludeTimeSeriesData: false,
		ExcludeDBData:         false,
		PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
			return []byte("mock output"), nil
		},
	}

	// Test the first job (exec-apigw-nginx-t)
	jobs := NIMJobList()
	execJob := jobs[0]

	ctx := context.Background()
	ch := make(chan JobResult, 1)
	execJob.Execute(dc, ctx, ch)

	result := <-ch
	logContent := logOutput.String()

	// Verify errors are logged for the failing namespaces
	assert.Contains(t, logContent, "Could not retrieve pod list for namespace error-ns1: network timeout")
	assert.Contains(t, logContent, "Could not retrieve pod list for namespace error-ns2: permission denied")
	assert.Contains(t, logContent, "Could not retrieve pod list for namespace error-ns3: resource not found")

	// success-ns should not have error logs
	assert.NotContains(t, logContent, "Could not retrieve pod list for namespace success-ns")

	// No files should be created since no apigw pods exist in success-ns
	assert.Empty(t, result.Files)
	assert.Nil(t, result.Error)
}

func TestNIMJobList_PodListError_LogFormat(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("specific error message for testing")
	})

	dc := &data_collector.DataCollector{
		BaseDir:               tmpDir,
		Namespaces:            []string{"test-ns"},
		Logger:                log.New(&logOutput, "", 0),
		K8sCoreClientSet:      client,
		ExcludeTimeSeriesData: false,
		ExcludeDBData:         false,
		PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	jobs := NIMJobList()
	execJob := jobs[0] // Test with first job

	ctx := context.Background()
	ch := make(chan JobResult, 1)
	execJob.Execute(dc, ctx, ch)

	<-ch
	logContent := logOutput.String()

	// Verify the exact log format
	expectedLogMessage := "\tCould not retrieve pod list for namespace test-ns: specific error message for testing"
	assert.Contains(t, logContent, expectedLogMessage)

	// Verify it starts with tab character for indentation
	assert.Contains(t, logContent, "\tCould not retrieve pod list")

	// Verify it contains the namespace and error
	assert.Contains(t, logContent, "test-ns")
	assert.Contains(t, logContent, "specific error message for testing")
}

func TestNIMJobList_ClickhouseData_SkippedVsPodListError(t *testing.T) {
	tests := []struct {
		name                  string
		excludeTimeSeriesData bool
		podListError          error
		expectSkipped         bool
		expectPodListError    bool
	}{
		{
			name:                  "excluded time series data - should skip",
			excludeTimeSeriesData: true,
			podListError:          nil,
			expectSkipped:         true,
			expectPodListError:    false,
		},
		{
			name:                  "pod list error - should log error",
			excludeTimeSeriesData: false,
			podListError:          fmt.Errorf("pod list API error"),
			expectSkipped:         false,
			expectPodListError:    true,
		},
		{
			name:                  "normal execution",
			excludeTimeSeriesData: false,
			podListError:          nil,
			expectSkipped:         false,
			expectPodListError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			client := fake.NewClientset()
			if tt.podListError != nil {
				client.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, tt.podListError
				})
			}

			dc := &data_collector.DataCollector{
				BaseDir:               tmpDir,
				Namespaces:            []string{"default"},
				Logger:                log.New(&logOutput, "", 0),
				K8sCoreClientSet:      client,
				ExcludeTimeSeriesData: tt.excludeTimeSeriesData,
				ExcludeDBData:         false,
				PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
					return []byte("clickhouse output"), nil
				},
			}

			// Get the exec-clickhouse-data job (index 3)
			jobs := NIMJobList()
			execJob := jobs[3]

			ctx := context.Background()
			ch := make(chan JobResult, 1)
			execJob.Execute(dc, ctx, ch)

			result := <-ch
			logContent := logOutput.String()

			if tt.expectSkipped {
				assert.True(t, result.Skipped, "Job should be skipped")
				assert.Contains(t, logContent, "Skipping clickhouse data dump as ExcludeTimeSeriesData is set to true")
			} else {
				assert.False(t, result.Skipped, "Job should not be skipped")
			}

			if tt.expectPodListError {
				assert.Contains(t, logContent, "Could not retrieve pod list for namespace default")
				assert.Contains(t, logContent, "pod list API error")
			} else if !tt.expectSkipped {
				assert.NotContains(t, logContent, "Could not retrieve pod list for namespace default")
			}
		})
	}
}

func TestNIMJobList_DqliteDump_SkippedVsPodListError(t *testing.T) {
	tests := []struct {
		name               string
		excludeDBData      bool
		podListError       error
		expectSkipped      bool
		expectPodListError bool
	}{
		{
			name:               "excluded DB data - should skip",
			excludeDBData:      true,
			podListError:       nil,
			expectSkipped:      true,
			expectPodListError: false,
		},
		{
			name:               "pod list error - should log error",
			excludeDBData:      false,
			podListError:       fmt.Errorf("pod list API error"),
			expectSkipped:      false,
			expectPodListError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			client := fake.NewClientset()
			if tt.podListError != nil {
				client.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, tt.podListError
				})
			}

			dc := &data_collector.DataCollector{
				BaseDir:               tmpDir,
				Namespaces:            []string{"default"},
				Logger:                log.New(&logOutput, "", 0),
				K8sCoreClientSet:      client,
				ExcludeTimeSeriesData: false,
				ExcludeDBData:         tt.excludeDBData,
				PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
					return []byte("dqlite output"), nil
				},
			}

			// Get the exec-dqlite-dump job (index 4)
			jobs := NIMJobList()
			execJob := jobs[4]

			ctx := context.Background()
			ch := make(chan JobResult, 1)
			execJob.Execute(dc, ctx, ch)

			result := <-ch
			logContent := logOutput.String()

			if tt.expectSkipped {
				assert.True(t, result.Skipped, "Job should be skipped")
				assert.Contains(t, logContent, "Skipping dqlite dump as ExcludeDBData is set to true")
			} else {
				assert.False(t, result.Skipped, "Job should not be skipped")
			}

			if tt.expectPodListError {
				assert.Contains(t, logContent, "Could not retrieve pod list for namespace default")
				assert.Contains(t, logContent, "pod list API error")
			} else if !tt.expectSkipped {
				assert.NotContains(t, logContent, "Could not retrieve pod list for namespace default")
			}
		})
	}
}
