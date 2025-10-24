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

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/mock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	mockClient := fake.NewClientset()

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

func TestCommonJobList_FileCreation(t *testing.T) {
	tests := []struct {
		name              string
		jobName           string
		jobIndex          int
		expectedFiles     []string
		setupMockObjects  func() []runtime.Object
		verifyFileContent func(t *testing.T, files map[string][]byte, tmpDir string)
	}{
		{
			name:     "pod-list creates pods.json files",
			jobName:  "pod-list",
			jobIndex: 0,
			expectedFiles: []string{
				"resources/default/pods.json",
				"resources/test-ns/pods.json",
			},
			setupMockObjects: func() []runtime.Object {
				return []runtime.Object{
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
					},
				}
			},
			verifyFileContent: func(t *testing.T, files map[string][]byte, tmpDir string) {
				// Verify JSON structure
				for path, content := range files {
					if strings.Contains(path, "pods.json") {
						var podList corev1.PodList
						err := json.Unmarshal(content, &podList)
						assert.NoError(t, err, "Should be valid JSON")
						assert.GreaterOrEqual(t, len(podList.Items), 0, "Should contain pod items")
					}
				}
			},
		},
		// Add more test cases for other jobs...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			// Setup mock objects
			mockObjects := tt.setupMockObjects()
			client := fake.NewSimpleClientset(mockObjects...)

			dc := &data_collector.DataCollector{
				BaseDir:          tmpDir,
				Namespaces:       []string{"default", "test-ns"},
				Logger:           log.New(&logOutput, "", 0),
				K8sCoreClientSet: client,
			}

			// Execute the specific job
			jobs := CommonJobList()
			job := jobs[tt.jobIndex]
			assert.Equal(t, tt.jobName, job.Name)

			ctx := context.Background()
			ch := make(chan JobResult, 1)
			job.Execute(dc, ctx, ch)

			result := <-ch

			// Verify expected number of files
			assert.Len(t, result.Files, len(tt.expectedFiles),
				"Should create expected number of files")

			// Verify each expected file exists
			for _, expectedFile := range tt.expectedFiles {
				expectedPath := filepath.Join(tmpDir, expectedFile)
				content, exists := result.Files[expectedPath]
				assert.True(t, exists, "Expected file should exist: %s", expectedFile)
				assert.NotEmpty(t, content, "File content should not be empty: %s", expectedFile)
			}

			// Custom content verification
			if tt.verifyFileContent != nil {
				tt.verifyFileContent(t, result.Files, tmpDir)
			}

			// Verify no errors for successful operations
			if len(tt.expectedFiles) > 0 {
				assert.Nil(t, result.Error, "Should not have errors for successful execution")
			}
		})
	}
}

func TestCommonJobList_ResourceListJobs(t *testing.T) {
	resourceTests := []struct {
		jobName      string
		jobIndex     int
		resourceType string
		fileName     string
		setupObjects func() []runtime.Object
	}{
		{
			jobName:      "pod-list",
			jobIndex:     0,
			resourceType: "pods",
			fileName:     "pods.json",
			setupObjects: func() []runtime.Object {
				return []runtime.Object{
					&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"}},
				}
			},
		},
		{
			jobName:      "service-list",
			jobIndex:     9,
			resourceType: "services",
			fileName:     "services.json",
			setupObjects: func() []runtime.Object {
				return []runtime.Object{
					&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"}},
				}
			},
		},
		{
			jobName:      "configmap-list",
			jobIndex:     8,
			resourceType: "configmaps",
			fileName:     "configmaps.json",
			setupObjects: func() []runtime.Object {
				return []runtime.Object{
					&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"}},
				}
			},
		},
	}

	for _, tt := range resourceTests {
		t.Run(tt.jobName, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			client := fake.NewSimpleClientset(tt.setupObjects()...)

			dc := &data_collector.DataCollector{
				BaseDir:          tmpDir,
				Namespaces:       []string{"default"},
				Logger:           log.New(&logOutput, "", 0),
				K8sCoreClientSet: client,
			}

			jobs := CommonJobList()
			job := jobs[tt.jobIndex]

			ctx := context.Background()
			ch := make(chan JobResult, 1)
			job.Execute(dc, ctx, ch)

			result := <-ch

			// Verify file creation
			expectedPath := filepath.Join(tmpDir, "resources/default", tt.fileName)
			content, exists := result.Files[expectedPath]
			assert.True(t, exists, "Expected %s file should exist", tt.fileName)
			assert.NotEmpty(t, content, "File content should not be empty")

			// Verify JSON structure
			var jsonData map[string]interface{}
			err := json.Unmarshal(content, &jsonData)
			assert.NoError(t, err, "Content should be valid JSON")
			assert.Contains(t, jsonData, "items", "Should contain 'items' field")
		})
	}
}

func TestCommonJobList_CollectPodsLogs_FileCreation(t *testing.T) {
	tmpDir := t.TempDir()
	var logOutput bytes.Buffer

	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx", Image: "nginx:latest"},
				{Name: "sidecar", Image: "busybox:latest"},
			},
		},
	}

	client := fake.NewSimpleClientset(testPod)

	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"default"},
		Logger:           log.New(&logOutput, "", 0),
		K8sCoreClientSet: client,
	}

	jobs := CommonJobList()
	collectLogsJob := jobs[1] // collect-pods-logs

	ctx := context.Background()
	ch := make(chan JobResult, 1)
	collectLogsJob.Execute(dc, ctx, ch)

	result := <-ch

	// Expected log files (will fail with fake client, but we can verify the attempt)
	expectedLogFiles := []string{
		"logs/default/nginx-pod__nginx.txt",
		"logs/default/nginx-pod__sidecar.txt",
	}

	// Verify expected file paths would be constructed correctly
	for _, expectedFile := range expectedLogFiles {
		expectedFullPath := filepath.Join(tmpDir, expectedFile)
		_, exists := result.Files[expectedFullPath]
		assert.True(t, exists, "Expected file should exist: %s", expectedFile)
	}
}

func TestCommonJobList_ClusterLevelJobs(t *testing.T) {
	clusterTests := []struct {
		name         string
		jobName      string
		jobIndex     int
		expectedFile string
	}{
		{
			name:         "k8s-version creates version.json",
			jobName:      "k8s-version",
			jobIndex:     18,
			expectedFile: "k8s/version.json",
		},
		{
			name:         "clusterroles-info creates clusterroles.json",
			jobName:      "clusterroles-info",
			jobIndex:     20,
			expectedFile: "k8s/rbac/clusterroles.json",
		},
		{
			name:         "nodes-info creates nodes.json and platform_info.json",
			jobName:      "nodes-info",
			jobIndex:     22,
			expectedFile: "k8s/nodes.json",
		},
	}

	for _, tt := range clusterTests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var logOutput bytes.Buffer

			// Setup mock objects based on job type
			var mockObjects []runtime.Object
			if tt.jobName == "nodes-info" {
				mockObjects = []runtime.Object{
					&corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-node",
							Labels: map[string]string{
								"node-role.kubernetes.io/control-plane": "",
							},
						},
						Status: corev1.NodeStatus{
							NodeInfo: corev1.NodeSystemInfo{
								OSImage:         "Ubuntu 20.04",
								OperatingSystem: "linux",
								Architecture:    "amd64",
							},
						},
					},
				}
			}

			client := fake.NewSimpleClientset(mockObjects...)

			dc := &data_collector.DataCollector{
				BaseDir:          tmpDir,
				Namespaces:       []string{"default"},
				Logger:           log.New(&logOutput, "", 0),
				K8sCoreClientSet: client,
			}

			jobs := CommonJobList()
			job := jobs[tt.jobIndex]
			assert.Equal(t, tt.jobName, job.Name)

			ctx := context.Background()
			ch := make(chan JobResult, 1)
			job.Execute(dc, ctx, ch)

			result := <-ch

			// Verify main file creation
			expectedPath := filepath.Join(tmpDir, tt.expectedFile)
			content, exists := result.Files[expectedPath]
			assert.True(t, exists, "Expected file should exist: %s", tt.expectedFile)
			assert.NotEmpty(t, content, "File content should not be empty")

			// Special case for nodes-info which creates additional platform_info.json
			if tt.jobName == "nodes-info" {
				platformInfoPath := filepath.Join(tmpDir, "platform_info.json")
				platformContent, platformExists := result.Files[platformInfoPath]
				assert.True(t, platformExists, "platform_info.json should exist")
				assert.NotEmpty(t, platformContent, "Platform info should not be empty")

				// Verify platform info structure
				var platformInfo data_collector.PlatformInfo
				err := json.Unmarshal(platformContent, &platformInfo)
				assert.NoError(t, err, "Platform info should be valid JSON")
				assert.NotEmpty(t, platformInfo.PlatformType, "Platform type should be set")
				assert.NotEmpty(t, platformInfo.Hostname, "Hostname should be set")
			}
		})
	}
}

func TestCommonJobList_AllJobsFileCreation(t *testing.T) {
	dc := mock.SetupMockDataCollector(t)
	jobs := CommonJobList()

	// Track all created files across all jobs
	allFiles := make(map[string][]byte)

	for i, job := range jobs {
		t.Run(fmt.Sprintf("job_%d_%s", i, job.Name), func(t *testing.T) {
			ctx := context.Background()
			ch := make(chan JobResult, 1)
			job.Execute(dc, ctx, ch)

			result := <-ch

			// Collect files from this job
			for path, content := range result.Files {
				allFiles[path] = content
			}

			// Verify files are within base directory
			for filePath := range result.Files {
				assert.True(t, strings.HasPrefix(filePath, dc.BaseDir),
					"File should be within base directory: %s", filePath)
			}
		})
	}

	// Verify overall file structure
	t.Run("verify_overall_structure", func(t *testing.T) {
		// Check that we have files in expected directories
		expectedDirs := []string{"resources", "k8s", "k8s/rbac"}

		for _, expectedDir := range expectedDirs {
			found := false
			for filePath := range allFiles {
				if strings.Contains(filePath, expectedDir) {
					found = true
					break
				}
			}
			assert.True(t, found, "Should have files in directory: %s", expectedDir)
		}

		// Verify minimum number of files created
		assert.Greater(t, len(allFiles), 10, "Should create multiple files across all jobs")
	})
}
