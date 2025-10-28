package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	dc.K8sCoreClientSet = fake.NewClientset(&corev1.Pod{
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

func TestNICJobList_CRDObjects_QueryFailure(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	dc := &data_collector.DataCollector{
		BaseDir:    tmpDir,
		Namespaces: []string{"default", "nginx-ingress"},
		Logger:     log.New(&logOutput, "", 0),
		QueryCRD: func(crd crds.Crd, namespace string, ctx context.Context) ([]byte, error) {
			// Return different errors based on CRD and namespace
			if namespace == "nginx-ingress" && crd.Resource == "virtualservers" {
				return nil, fmt.Errorf("permission denied")
			}
			if namespace == "default" && crd.Resource == "policies" {
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

	// Get the crd-objects job (index 4)
	jobs := NICJobList()
	crdJob := jobs[4]
	assert.Equal(t, "crd-objects", crdJob.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := make(chan JobResult, 1)
	go crdJob.Execute(dc, ctx, ch)

	select {
	case result := <-ch:
		logContent := logOutput.String()

		// Verify error logging for specific failures
		assert.Contains(t, logContent, "CRD virtualservers.k8s.nginx.org/v1 could not be collected in namespace nginx-ingress: permission denied")
		assert.Contains(t, logContent, "CRD policies.k8s.nginx.org/v1 could not be collected in namespace default: resource not found")

		// Verify successful CRDs still created files (only failures are logged)
		expectedCRDs := crds.GetNICCRDList()
		successfulFiles := 0

		for _, namespace := range dc.Namespaces {
			for _, crd := range expectedCRDs {
				// Skip the ones we know should fail
				if (namespace == "nginx-ingress" && crd.Resource == "virtualservers") ||
					(namespace == "default" && crd.Resource == "policies") {
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

	case <-ctx.Done():
		t.Fatal("CRD job timed out")
	}
}

func TestNICJobList_PodListFailure_AllJobs(t *testing.T) {
	tests := []struct {
		name     string
		jobName  string
		jobIndex int
	}{
		{
			name:     "exec-nginx-ingress-version pod list failure",
			jobName:  "exec-nginx-ingress-version",
			jobIndex: 0,
		},
		{
			name:     "exec-nginx-t pod list failure",
			jobName:  "exec-nginx-t",
			jobIndex: 1,
		},
		{
			name:     "exec-agent-conf pod list failure",
			jobName:  "exec-agent-conf",
			jobIndex: 2,
		},
		{
			name:     "exec-agent-version pod list failure",
			jobName:  "exec-agent-version",
			jobIndex: 3,
		},
		{
			name:     "collect-product-platform-info pod list failure",
			jobName:  "collect-product-platform-info",
			jobIndex: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("Running subtest: %s\n", tt.name)
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			// Create a fake client that will return an error for pod listing
			client := fake.NewSimpleClientset()
			client.PrependReactor("list", "pods", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, fmt.Errorf("failed to retrieve pod list")
			})

			dc := &data_collector.DataCollector{
				BaseDir:          tmpDir,
				Namespaces:       []string{"default", "nginx-ingress"},
				Logger:           log.New(&logOutput, "", 0),
				K8sCoreClientSet: client,
				PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
					return []byte("mock output"), nil
				},
			}

			// Get the specific job
			jobs := NICJobList()
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
			assert.Contains(t, logContent, "Could not retrieve pod list for namespace nginx-ingress: failed to retrieve pod list")

			// Verify no files were created since pod listing failed
			assert.Empty(t, result.Files, "No files should be created when pod list fails")
			assert.Nil(t, result.Error, "Job should not fail, just log the error")
		})
	}
}

func TestNICJobList_PodListFailure_vs_CommandExecutionFailure(t *testing.T) {
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

			// Create nginx ingress pod for testing
			nginxIngressPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-ingress-controller-123",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nginx-ingress", Image: "nginx-ingress:latest"},
					},
				},
			}

			client := fake.NewSimpleClientset(nginxIngressPod)

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
					return []byte("nginx ingress version output"), nil
				},
			}

			jobs := NICJobList()
			execJob := jobs[0] // exec-nginx-ingress-version

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

			// Verify error state
			if tt.executionError != nil {
				assert.NotNil(t, result.Error, "Should have error when execution fails")
			} else {
				assert.Nil(t, result.Error, "Should not have error when execution succeeds")
			}
		})
	}
}

func TestNICJobList_CommandExecutionFailure(t *testing.T) {
	tests := []struct {
		name              string
		jobName           string
		jobIndex          int
		expectedCommand   []string
		expectedContainer string
		expectedFileExt   string
	}{
		{
			name:              "exec-nginx-ingress-version command failure",
			jobName:           "exec-nginx-ingress-version",
			jobIndex:          0,
			expectedCommand:   []string{"./nginx-ingress", "--version"},
			expectedContainer: "nginx-ingress",
			expectedFileExt:   "__nginx-ingress-version.txt",
		},
		{
			name:              "exec-nginx-t command failure",
			jobName:           "exec-nginx-t",
			jobIndex:          1,
			expectedCommand:   []string{"/usr/sbin/nginx", "-T"},
			expectedContainer: "nginx-ingress",
			expectedFileExt:   "__nginx-t.txt",
		},
		{
			name:              "exec-agent-conf command failure",
			jobName:           "exec-agent-conf",
			jobIndex:          2,
			expectedCommand:   []string{"cat", "/etc/nginx-agent/nginx-agent.conf"},
			expectedContainer: "nginx-ingress",
			expectedFileExt:   "__nginx-agent.conf",
		},
		{
			name:              "exec-agent-version command failure",
			jobName:           "exec-agent-version",
			jobIndex:          3,
			expectedCommand:   []string{"/usr/bin/nginx-agent", "--version"},
			expectedContainer: "nginx-ingress",
			expectedFileExt:   "__nginx-agent-version.txt",
		},
		{
			name:              "collect-product-platform-info command failure",
			jobName:           "collect-product-platform-info",
			jobIndex:          5,
			expectedCommand:   []string{"./nginx-ingress", "--version"},
			expectedContainer: "nginx-ingress",
			expectedFileExt:   "__nginx-ingress-version.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			// Create nginx-ingress pod for testing
			nginxIngressPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-ingress-controller-123",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nginx-ingress", Image: "nginx-ingress:latest"},
					},
				},
			}

			client := fake.NewSimpleClientset(nginxIngressPod)

			dc := &data_collector.DataCollector{
				BaseDir:          tmpDir,
				Namespaces:       []string{"default"},
				Logger:           log.New(&logOutput, "", 0),
				K8sCoreClientSet: client,
				PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
					// Verify correct parameters are passed
					assert.Equal(t, "default", namespace)
					assert.Equal(t, "nginx-ingress-controller-123", podName)
					assert.Equal(t, tt.expectedContainer, containerName)
					assert.Equal(t, tt.expectedCommand, command)

					// Return error to test failure path
					return nil, fmt.Errorf("command execution failed: %v", command)
				},
			}

			// Execute the specific job
			jobs := NICJobList()
			job := jobs[tt.jobIndex]
			assert.Equal(t, tt.jobName, job.Name)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			ch := make(chan JobResult, 1)
			go job.Execute(dc, ctx, ch)

			select {
			case result := <-ch:
				logContent := logOutput.String()

				// Verify the error was set
				assert.NotNil(t, result.Error, "Job should have error when command execution fails")
				assert.Contains(t, result.Error.Error(), "command execution failed")

				// Verify the error was logged
				expectedLogMessage := fmt.Sprintf("Command execution %s failed for pod nginx-ingress-controller-123 in namespace default", tt.expectedCommand)
				assert.Contains(t, logContent, expectedLogMessage)
				assert.Contains(t, logContent, "command execution failed")

				// Verify no files were created when command execution fails
				assert.Empty(t, result.Files, "No files should be created when command execution fails")

			case <-ctx.Done():
				t.Fatalf("Job %s timed out", tt.jobName)
			}
		})
	}
}

func TestNICJobList_CollectProductPlatformInfo_JSONMarshalFailure(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	nginxIngressPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-ingress-controller",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx-ingress", Image: "nginx-ingress:latest"},
			},
		},
	}

	client := fake.NewSimpleClientset(nginxIngressPod)

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"default"},
		Logger:           log.New(&logOutput, "", 0),
		K8sCoreClientSet: client,
		PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
			// Return successful command output
			return []byte("Version=1.2.3 Commit=abc123"), nil
		},
	}

	// Test the collect-product-platform-info job (index 5)
	jobs := NICJobList()
	job := jobs[5]
	assert.Equal(t, "collect-product-platform-info", job.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := make(chan JobResult, 1)
	go job.Execute(dc, ctx, ch)

	select {
	case result := <-ch:
		// This should succeed since JSON marshaling should work
		assert.Nil(t, result.Error, "Should not have JSON marshal error with valid data")
		assert.Len(t, result.Files, 1, "Should create product_info.json file")

		// Verify product_info.json was created
		expectedPath := filepath.Join(tmpDir, "product_info.json")
		content, exists := result.Files[expectedPath]
		assert.True(t, exists, "product_info.json should exist")

		// Verify JSON structure
		var productInfo data_collector.ProductInfo
		err := json.Unmarshal(content, &productInfo)
		assert.NoError(t, err, "Should be valid JSON")
		assert.Equal(t, "1.2.3", productInfo.Version)
		assert.Equal(t, "abc123", productInfo.Build)
		assert.Equal(t, "NGINX Ingress Controller", productInfo.Product)

	case <-ctx.Done():
		t.Fatal("Job timed out")
	}
}
