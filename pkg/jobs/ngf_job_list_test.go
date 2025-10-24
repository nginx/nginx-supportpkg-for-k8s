package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/crds"
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

func TestNGFJobList_PodListFailure(t *testing.T) {
	tests := []struct {
		name     string
		jobName  string
		jobIndex int
	}{
		{
			name:     "exec-nginx-gateway-version pod list failure",
			jobName:  "exec-nginx-gateway-version",
			jobIndex: 0,
		},
		{
			name:     "exec-nginx-t pod list failure",
			jobName:  "exec-nginx-t",
			jobIndex: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			// Create a fake client that will return an error for pod listing
			client := fake.NewSimpleClientset()
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

			// Get the specific job
			jobs := NGFJobList()
			job := jobs[tt.jobIndex]
			assert.Equal(t, tt.jobName, job.Name, "Job name should match expected")

			// Execute the job
			ctx := context.Background()
			ch := make(chan JobResult, 1)
			job.Execute(dc, ctx, ch)

			result := <-ch
			logContent := logOutput.String()

			// Verify the error was logged for each namespace
			assert.Contains(t, logContent, "Could not retrieve pod list for namespace default: failed to retrieve pod list")
			assert.Contains(t, logContent, "Could not retrieve pod list for namespace nginx-gateway: failed to retrieve pod list")

			// Verify no files were created since pod listing failed
			assert.Empty(t, result.Files, "No files should be created when pod list fails")
			assert.Nil(t, result.Error, "Job should not fail, just log the error")
		})
	}
}

func TestNGFJobList_PodListFailure_MultipleNamespaces(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	// Create a fake client that returns different errors for different namespaces
	client := fake.NewSimpleClientset()
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
			// Let other namespaces succeed (but with no nginx-gateway pods)
			return false, nil, nil
		}
	})

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"error-ns1", "error-ns2", "error-ns3", "success-ns"},
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
			assert.Contains(t, logContent, "Could not retrieve pod list for namespace error-ns3: resource not found")

			// success-ns should not have error logs
			assert.NotContains(t, logContent, "Could not retrieve pod list for namespace success-ns")

			// No files should be created since no nginx-gateway pods exist in success-ns
			assert.Empty(t, result.Files)
			assert.Nil(t, result.Error)
		})
	}
}

func TestNGFJobList_CommandExecutionFailure(t *testing.T) {
	tests := []struct {
		name              string
		jobName           string
		jobIndex          int
		expectedCommand   []string
		expectedContainer string
		expectedFileExt   string
	}{
		{
			name:              "exec-nginx-gateway-version command failure",
			jobName:           "exec-nginx-gateway-version",
			jobIndex:          0,
			expectedCommand:   []string{"/usr/bin/gateway", "--help"},
			expectedContainer: "nginx-gateway",
			expectedFileExt:   "__nginx-gateway-version.txt",
		},
		{
			name:              "exec-nginx-t command failure",
			jobName:           "exec-nginx-t",
			jobIndex:          1,
			expectedCommand:   []string{"/usr/sbin/nginx", "-T"},
			expectedContainer: "nginx",
			expectedFileExt:   "__nginx-t.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			// Create nginx-gateway pod for testing
			nginxGatewayPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-gateway-deployment-123",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nginx-gateway", Image: "nginx-gateway:latest"},
						{Name: "nginx", Image: "nginx:latest"},
					},
				},
			}

			client := fake.NewSimpleClientset(nginxGatewayPod)

			dc := &data_collector.DataCollector{
				BaseDir:          tmpDir,
				Namespaces:       []string{"default"},
				Logger:           log.New(&logOutput, "", 0),
				K8sCoreClientSet: client,
				PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
					// Verify correct parameters are passed
					assert.Equal(t, "default", namespace)
					assert.Equal(t, "nginx-gateway-deployment-123", podName)
					assert.Equal(t, tt.expectedContainer, containerName)
					assert.Equal(t, tt.expectedCommand, command)

					// Return error to test failure path
					return nil, fmt.Errorf("command execution failed: %v", command)
				},
			}

			// Execute the specific job
			jobs := NGFJobList()
			job := jobs[tt.jobIndex]
			assert.Equal(t, tt.jobName, job.Name)

			ctx := context.Background()
			ch := make(chan JobResult, 1)
			job.Execute(dc, ctx, ch)

			result := <-ch
			logContent := logOutput.String()

			// Verify the error was set
			assert.NotNil(t, result.Error, "Job should have error when command execution fails")
			assert.Contains(t, result.Error.Error(), "command execution failed")

			// Verify the error was logged
			expectedLogMessage := fmt.Sprintf("Command execution %s failed for pod nginx-gateway-deployment-123 in namespace default", tt.expectedCommand)
			assert.Contains(t, logContent, expectedLogMessage)
			assert.Contains(t, logContent, "command execution failed")

			// Verify no files were created when command execution fails
			assert.Empty(t, result.Files, "No files should be created when command execution fails")
		})
	}
}

func TestNGFJobList_CRDObjects_Success(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	dc := &data_collector.DataCollector{
		BaseDir:    tmpDir,
		Namespaces: []string{"default", "nginx-gateway"},
		Logger:     log.New(&logOutput, "", 0),
		QueryCRD: func(crd crds.Crd, namespace string, ctx context.Context) ([]byte, error) {
			// Mock successful CRD query
			mockData := map[string]interface{}{
				"apiVersion": crd.Group + "/" + crd.Version,
				"kind":       crd.Resource,
				"items": []map[string]interface{}{
					{
						"metadata": map[string]interface{}{
							"name":      "test-" + crd.Resource,
							"namespace": namespace,
						},
						"spec": map[string]interface{}{
							"host": "example.com",
						},
					},
				},
			}
			return json.Marshal(mockData)
		},
	}

	// Get the crd-objects job
	jobs := NGFJobList()
	crdJob := jobs[2] // crd-objects is at index 2
	assert.Equal(t, "crd-objects", crdJob.Name)

	ctx := context.Background()
	ch := make(chan JobResult, 1)
	crdJob.Execute(dc, ctx, ch)

	result := <-ch

	// Verify no errors
	assert.Nil(t, result.Error, "Should not have errors for successful CRD collection")

	// Get the expected CRDs from GetNGFCRDList()
	expectedCRDs := crds.GetNGFCRDList()
	expectedFileCount := len(expectedCRDs) * len(dc.Namespaces)

	// Verify expected number of files created
	assert.Len(t, result.Files, expectedFileCount,
		"Should create files for each CRD in each namespace")

	// Verify file paths and content for each CRD and namespace
	for _, namespace := range dc.Namespaces {
		for _, crd := range expectedCRDs {
			expectedPath := filepath.Join(tmpDir, "crds", namespace, crd.Resource+".json")
			content, exists := result.Files[expectedPath]

			assert.True(t, exists, "File should exist for CRD %s in namespace %s", crd.Resource, namespace)
			assert.NotEmpty(t, content, "File content should not be empty")

			// Verify JSON structure
			var jsonData map[string]interface{}
			err := json.Unmarshal(content, &jsonData)
			assert.NoError(t, err, "Content should be valid JSON")
			assert.Contains(t, jsonData, "items", "Should contain items field")
		}
	}

	// Verify no error messages in logs
	logContent := logOutput.String()
	assert.NotContains(t, logContent, "could not be collected", "Should not have CRD collection errors")
}

func TestNGFJobList_CRDObjects_QueryFailure(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	dc := &data_collector.DataCollector{
		BaseDir:    tmpDir,
		Namespaces: []string{"default", "test-ns"},
		Logger:     log.New(&logOutput, "", 0),
		QueryCRD: func(crd crds.Crd, namespace string, ctx context.Context) ([]byte, error) {
			// Return different errors based on CRD and namespace
			if namespace == "test-ns" && crd.Resource == "nginxgateways" {
				return nil, fmt.Errorf("permission denied")
			}
			if namespace == "default" && crd.Resource == "clientsettingspolicies" {
				return nil, fmt.Errorf("resource not found")
			}

			// Success for other combinations
			mockData := map[string]interface{}{
				"apiVersion": crd.Group + "/" + crd.Version,
				"kind":       crd.Resource,
				"items":      []interface{}{},
			}
			return json.Marshal(mockData)
		},
	}

	jobs := NGFJobList()
	crdJob := jobs[2]

	ctx := context.Background()
	ch := make(chan JobResult, 1)
	crdJob.Execute(dc, ctx, ch)

	result := <-ch
	logContent := logOutput.String()

	// Verify error logging for specific failures
	assert.Contains(t, logContent, "CRD nginxgateways.gateway.nginx.org/v1alpha1 could not be collected in namespace test-ns: permission denied")
	assert.Contains(t, logContent, "CRD clientsettingspolicies.gateway.nginx.org/v1alpha1 could not be collected in namespace default: resource not found")

	// Verify successful CRDs still created files (only failures are logged)
	expectedCRDs := crds.GetNGFCRDList()
	successfulFiles := 0

	for _, namespace := range dc.Namespaces {
		for _, crd := range expectedCRDs {
			// Skip the ones we know should fail
			if (namespace == "test-ns" && crd.Resource == "gateways") ||
				(namespace == "default" && crd.Resource == "httproutes") {
				continue
			}

			expectedPath := filepath.Join(tmpDir, "crds", namespace, crd.Resource+".json")
			_, exists := result.Files[expectedPath]
			if exists {
				successfulFiles++
			}
		}
	}

	assert.Greater(t, successfulFiles, 0, "Should have some successful CRD files")
	assert.Nil(t, result.Error, "Job should not fail even if some CRDs fail to collect")
}
