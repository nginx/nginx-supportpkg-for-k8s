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

func TestNGXJobList_ExecNginxT_CreatesExpectedFiles(t *testing.T) {
	tests := []struct {
		name          string
		pods          []*corev1.Pod
		namespaces    []string
		expectedFiles []string
		podExecutor   func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error)
	}{
		{
			name:       "single nginx pod creates one file",
			namespaces: []string{"default"},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx-deployment-123",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "nginx", Image: "nginx:latest"},
						},
					},
				},
			},
			expectedFiles: []string{
				"exec/default/nginx-deployment-123__nginx-t.txt",
			},
			podExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
				return []byte("nginx configuration output"), nil
			},
		},
		{
			name:       "multiple nginx pods create multiple files",
			namespaces: []string{"default", "production"},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx-web-1",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "nginx", Image: "nginx:latest"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx-api-2",
						Namespace: "production",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "nginx", Image: "nginx:latest"},
						},
					},
				},
			},
			expectedFiles: []string{
				"exec/default/nginx-web-1__nginx-t.txt",
				"exec/production/nginx-api-2__nginx-t.txt",
			},
			podExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
				return []byte(fmt.Sprintf("nginx config for %s/%s", namespace, podName)), nil
			},
		},
		{
			name:       "non-nginx pods create no files",
			namespaces: []string{"default"},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "redis-deployment-456",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "redis", Image: "redis:latest"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "postgres-db-789",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "postgres", Image: "postgres:latest"},
						},
					},
				},
			},
			expectedFiles: []string{}, // No files expected for non-nginx pods
			podExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
				return []byte("should not be called"), nil
			},
		},
		{
			name:       "mixed pods only create files for nginx",
			namespaces: []string{"default"},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx-proxy-1",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "nginx", Image: "nginx:latest"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "redis-cache-2",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "redis", Image: "redis:latest"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx-ingress-3",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "nginx", Image: "nginx:latest"},
						},
					},
				},
			},
			expectedFiles: []string{
				"exec/default/nginx-proxy-1__nginx-t.txt",
				"exec/default/nginx-ingress-3__nginx-t.txt",
			},
			podExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
				return []byte(fmt.Sprintf("nginx config for %s", podName)), nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			// Create fake client with test pods
			var runtimeObjects []runtime.Object
			for _, pod := range tt.pods {
				runtimeObjects = append(runtimeObjects, pod)
			}
			client := fake.NewSimpleClientset(runtimeObjects...)

			dc := &data_collector.DataCollector{
				BaseDir:          tmpDir,
				Namespaces:       tt.namespaces,
				Logger:           log.New(&logOutput, "", 0),
				K8sCoreClientSet: client,
				PodExecutor:      tt.podExecutor,
			}

			// Execute the job
			jobs := NGXJobList()
			execJob := jobs[0] // exec-nginx-t

			ctx := context.Background()
			ch := make(chan JobResult, 1)
			execJob.Execute(dc, ctx, ch)

			result := <-ch

			// Verify the number of files created
			assert.Len(t, result.Files, len(tt.expectedFiles), "Number of files should match expected")

			// Verify each expected file is created
			for _, expectedFile := range tt.expectedFiles {
				expectedPath := filepath.Join(tmpDir, expectedFile)
				content, exists := result.Files[expectedPath]
				assert.True(t, exists, "Expected file should exist: %s", expectedFile)
				assert.NotEmpty(t, content, "File content should not be empty for: %s", expectedFile)

				// Verify content contains expected data
				contentStr := string(content)
				if strings.Contains(expectedFile, "nginx-web-1") {
					assert.Contains(t, contentStr, "default/nginx-web-1")
				} else if strings.Contains(expectedFile, "nginx-api-2") {
					assert.Contains(t, contentStr, "production/nginx-api-2")
				}
			}

			// Verify no unexpected files are created
			for filePath := range result.Files {
				relativePath, err := filepath.Rel(tmpDir, filePath)
				assert.NoError(t, err)
				assert.Contains(t, tt.expectedFiles, relativePath, "Unexpected file created: %s", relativePath)
			}

			// Verify no errors if execution was successful
			if len(tt.expectedFiles) > 0 {
				assert.Nil(t, result.Error, "Should not have errors for successful execution")
			}
		})
	}
}

func TestNGXJobList_ExecNginxT_FileContents(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	nginxPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx", Image: "nginx:latest"},
			},
		},
	}

	client := fake.NewSimpleClientset(nginxPod)

	expectedConfig := `server {
    listen 80;
    server_name example.com;
    location / {
        proxy_pass http://backend;
    }
}`

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"default"},
		Logger:           log.New(&logOutput, "", 0),
		K8sCoreClientSet: client,
		PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
			// Verify the correct command is passed
			assert.Equal(t, []string{"/usr/sbin/nginx", "-T"}, command)
			assert.Equal(t, "default", namespace)
			assert.Equal(t, "nginx-test-pod", podName)
			assert.Equal(t, "nginx", containerName)

			return []byte(expectedConfig), nil
		},
	}

	jobs := NGXJobList()
	execJob := jobs[0]

	ctx := context.Background()
	ch := make(chan JobResult, 1)
	execJob.Execute(dc, ctx, ch)

	result := <-ch

	// Verify file is created with correct content
	assert.Len(t, result.Files, 1)

	expectedPath := filepath.Join(tmpDir, "exec/default/nginx-test-pod__nginx-t.txt")
	content, exists := result.Files[expectedPath]
	assert.True(t, exists, "Expected file should exist")
	assert.Equal(t, expectedConfig, string(content), "File content should match expected nginx config")
}

func TestNGXJobList_ExecNginxT_FilePaths(t *testing.T) {
	tests := []struct {
		name         string
		podName      string
		namespace    string
		expectedPath string
	}{
		{
			name:         "standard pod name",
			podName:      "nginx-deployment-abc123",
			namespace:    "default",
			expectedPath: "exec/default/nginx-deployment-abc123__nginx-t.txt",
		},
		{
			name:         "pod with dashes",
			podName:      "nginx-ingress-controller-xyz",
			namespace:    "ingress-nginx",
			expectedPath: "exec/ingress-nginx/nginx-ingress-controller-xyz__nginx-t.txt",
		},
		{
			name:         "short pod name",
			podName:      "nginx-1",
			namespace:    "prod",
			expectedPath: "exec/prod/nginx-1__nginx-t.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			nginxPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.podName,
					Namespace: tt.namespace,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nginx", Image: "nginx:latest"},
					},
				},
			}

			client := fake.NewSimpleClientset(nginxPod)

			dc := &data_collector.DataCollector{
				BaseDir:          tmpDir,
				Namespaces:       []string{tt.namespace},
				Logger:           log.New(&logOutput, "", 0),
				K8sCoreClientSet: client,
				PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
					return []byte("nginx config"), nil
				},
			}

			jobs := NGXJobList()
			execJob := jobs[0]

			ctx := context.Background()
			ch := make(chan JobResult, 1)
			execJob.Execute(dc, ctx, ch)

			result := <-ch

			// Verify the file path is constructed correctly
			expectedFullPath := filepath.Join(tmpDir, tt.expectedPath)
			content, exists := result.Files[expectedFullPath]
			assert.True(t, exists, "File should exist at expected path: %s", tt.expectedPath)
			assert.Equal(t, "nginx config", string(content))
		})
	}
}

func TestNGXJobList_ExecNginxT_NoFilesOnError(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	nginxPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx", Image: "nginx:latest"},
			},
		},
	}

	client := fake.NewSimpleClientset(nginxPod)

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"default"},
		Logger:           log.New(&logOutput, "", 0),
		K8sCoreClientSet: client,
		PodExecutor: func(namespace, podName, containerName string, command []string, ctx context.Context) ([]byte, error) {
			return nil, fmt.Errorf("command execution failed")
		},
	}

	jobs := NGXJobList()
	execJob := jobs[0]

	ctx := context.Background()
	ch := make(chan JobResult, 1)
	execJob.Execute(dc, ctx, ch)

	result := <-ch

	// Verify no files are created when command execution fails
	assert.Empty(t, result.Files, "No files should be created when command execution fails")
	assert.NotNil(t, result.Error, "Error should be set when command execution fails")

	// Verify error is logged
	logContent := logOutput.String()
	assert.Contains(t, logContent, "Command execution")
	assert.Contains(t, logContent, "failed for pod nginx-pod")
	assert.Contains(t, logContent, "command execution failed")
}
