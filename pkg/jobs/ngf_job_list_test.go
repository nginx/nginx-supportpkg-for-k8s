package jobs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/mock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
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
	client := fake.NewClientset(&corev1.Pod{
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

	dc := mock.SetupMockDataCollector(t)
	dc.K8sCoreClientSet = client
	dc.PodExecutor = func(namespace, pod, container string, command []string, ctx context.Context) ([]byte, error) {
		return []byte("gateway version output"), nil
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
	client := fake.NewClientset(&corev1.Pod{
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

	dc := mock.SetupMockDataCollector(t)
	dc.Namespaces = []string{"default"}
	dc.K8sCoreClientSet = client
	dc.PodExecutor = func(namespace, pod, container string, command []string, ctx context.Context) ([]byte, error) {
		return []byte("nginx -t output"), nil
	}
	dc.Logger = log.New(io.Discard, "", 0)

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

func TestNGFJobList_ExecNginxGatewayVersion_PodListError(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	// Create a fake client that will return an error for pod listing
	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("failed to retrieve pod list")
	})

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"default", "nginx-gateway"},
		Logger:           log.New(&logOutput, "", 0),
		K8sCoreClientSet: client,
		PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
			return []byte("mock output"), nil
		},
	}

	// Get the exec-nginx-gateway-version job
	jobs := NGFJobList()
	var execJob Job
	for _, job := range jobs {
		if job.Name == "exec-nginx-gateway-version" {
			execJob = job
			break
		}
	}

	// Execute the job
	ctx := context.Background()
	ch := make(chan JobResult, 1)
	execJob.Execute(dc, ctx, ch)

	result := <-ch
	logContent := logOutput.String()

	// Verify the error was logged for each namespace
	assert.Contains(t, logContent, "Could not retrieve pod list for namespace default: failed to retrieve pod list")
	assert.Contains(t, logContent, "Could not retrieve pod list for namespace nginx-gateway: failed to retrieve pod list")

	// Verify no files were created since pod listing failed
	assert.Empty(t, result.Files, "No files should be created when pod list fails")
	assert.Nil(t, result.Error, "Job should not fail, just log the error")
}

func TestNGFJobList_ExecNginxT_PodListError(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	// Create a fake client that will return an error for pod listing
	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("pod list API error")
	})

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"test-namespace"},
		Logger:           log.New(&logOutput, "", 0),
		K8sCoreClientSet: client,
		PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
			return []byte("mock nginx config"), nil
		},
	}

	// Get the exec-nginx-t job
	jobs := NGFJobList()
	var execJob Job
	for _, job := range jobs {
		if job.Name == "exec-nginx-t" {
			execJob = job
			break
		}
	}

	// Execute the job
	ctx := context.Background()
	ch := make(chan JobResult, 1)
	execJob.Execute(dc, ctx, ch)

	result := <-ch
	logContent := logOutput.String()

	// Verify the error was logged
	assert.Contains(t, logContent, "Could not retrieve pod list for namespace test-namespace: pod list API error")
	assert.Empty(t, result.Files, "No files should be created when pod list fails")
	assert.Nil(t, result.Error, "Job should not fail, just log the error")
}

func TestNGFJobList_MultipleNamespaces_PodListErrors(t *testing.T) {
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
		default:
			// Let other namespaces succeed
			return false, nil, nil
		}
	})

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"error-ns1", "error-ns2", "success-ns"},
		Logger:           log.New(&logOutput, "", 0),
		K8sCoreClientSet: client,
		PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
			return []byte("mock output"), nil
		},
	}

	// Test both jobs that have the same error handling pattern
	jobs := NGFJobList()

	for _, jobName := range []string{"exec-nginx-gateway-version", "exec-nginx-t"} {
		t.Run(jobName, func(t *testing.T) {
			var targetJob Job
			for _, job := range jobs {
				if job.Name == jobName {
					targetJob = job
					break
				}
			}

			// Clear log output for this subtest
			logOutput.Reset()

			ctx := context.Background()
			ch := make(chan JobResult, 1)
			targetJob.Execute(dc, ctx, ch)

			result := <-ch
			logContent := logOutput.String()

			// Verify errors are logged for the failing namespaces
			assert.Contains(t, logContent, "Could not retrieve pod list for namespace error-ns1: network timeout")
			assert.Contains(t, logContent, "Could not retrieve pod list for namespace error-ns2: permission denied")

			// success-ns should not have error logs
			assert.NotContains(t, logContent, "Could not retrieve pod list for namespace success-ns")

			// No files should be created since no nginx-gateway pods exist in success-ns
			assert.Empty(t, result.Files)
			assert.Nil(t, result.Error)
		})
	}
}

func TestNGFJobList_PodListError_LogFormat(t *testing.T) {
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

	jobs := NGFJobList()
	execJob := jobs[0] // exec-nginx-gateway-version

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
