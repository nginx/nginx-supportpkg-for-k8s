package jobs

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"log"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/mock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
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
	dc.K8sCoreClientSet = fake.NewClientset(pod)

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

func TestNGXJobList_ExecNginxT_PodListError(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	// Create a fake client that will return an error for pod listing
	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("failed to retrieve pod list")
	})

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"default", "nginx-ingress"},
		Logger:           log.New(&logOutput, "", 0),
		K8sCoreClientSet: client,
		PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
			return []byte("mock nginx config"), nil
		},
	}

	// Get the exec-nginx-t job
	jobs := NGXJobList()
	execJob := jobs[0] // exec-nginx-t is the only job

	// Execute the job
	ctx := context.Background()
	ch := make(chan JobResult, 1)
	execJob.Execute(dc, ctx, ch)

	result := <-ch
	logContent := logOutput.String()

	// Verify the error was logged for each namespace
	assert.Contains(t, logContent, "Could not retrieve pod list for namespace default: failed to retrieve pod list")
	assert.Contains(t, logContent, "Could not retrieve pod list for namespace nginx-ingress: failed to retrieve pod list")

	// Verify no files were created since pod listing failed
	assert.Empty(t, result.Files, "No files should be created when pod list fails")
	assert.Nil(t, result.Error, "Job should not fail, just log the error")
}

func TestNGXJobList_MultipleNamespaces_PodListErrors(t *testing.T) {
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
			// Let other namespaces succeed (but with no nginx pods)
			return false, nil, nil
		}
	})

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"error-ns1", "error-ns2", "error-ns3", "success-ns"},
		Logger:           log.New(&logOutput, "", 0),
		K8sCoreClientSet: client,
		PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
			return []byte("nginx config output"), nil
		},
	}

	jobs := NGXJobList()
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

	// No files should be created since no nginx pods exist in success-ns
	assert.Empty(t, result.Files)
	assert.Nil(t, result.Error)
}

func TestNGXJobList_PodListError_LogFormat(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("specific error message for testing")
	})

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"test-ns"},
		Logger:           log.New(&logOutput, "", 0),
		K8sCoreClientSet: client,
		PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	jobs := NGXJobList()
	execJob := jobs[0]

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

func TestNGXJobList_PodListError_vs_ExecutionError(t *testing.T) {
	tests := []struct {
		name           string
		podListError   error
		executionError error
		expectPodLog   bool
		expectExecLog  bool
		expectedFiles  int
	}{
		{
			name:           "pod list fails",
			podListError:   fmt.Errorf("pod list API error"),
			executionError: nil,
			expectPodLog:   true,
			expectExecLog:  false,
			expectedFiles:  0,
		},
		{
			name:           "pod list succeeds but execution fails",
			podListError:   nil,
			executionError: fmt.Errorf("command execution failed"),
			expectPodLog:   false,
			expectExecLog:  true,
			expectedFiles:  0,
		},
		{
			name:           "both succeed",
			podListError:   nil,
			executionError: nil,
			expectPodLog:   false,
			expectExecLog:  false,
			expectedFiles:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			// Create nginx pod for testing
			nginxPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-deployment-123",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nginx", Image: "nginx:latest"},
					},
				},
			}

			client := fake.NewClientset(nginxPod)

			if tt.podListError != nil {
				client.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, tt.podListError
				})
			}

			dc := &data_collector.DataCollector{
				BaseDir:          tmpDir,
				Namespaces:       []string{"default"},
				Logger:           log.New(&logOutput, "", 0),
				K8sCoreClientSet: client,
				PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
					if tt.executionError != nil {
						return nil, tt.executionError
					}
					return []byte("nginx configuration"), nil
				},
			}

			jobs := NGXJobList()
			execJob := jobs[0]

			ctx := context.Background()
			ch := make(chan JobResult, 1)
			execJob.Execute(dc, ctx, ch)

			result := <-ch
			logContent := logOutput.String()

			if tt.expectPodLog {
				assert.Contains(t, logContent, "Could not retrieve pod list for namespace default")
			} else {
				assert.NotContains(t, logContent, "Could not retrieve pod list for namespace default")
			}

			if tt.expectExecLog {
				assert.Contains(t, logContent, "Command execution")
				assert.Contains(t, logContent, "failed for pod")
			} else {
				assert.NotContains(t, logContent, "Command execution")
			}

			assert.Len(t, result.Files, tt.expectedFiles)
		})
	}
}
