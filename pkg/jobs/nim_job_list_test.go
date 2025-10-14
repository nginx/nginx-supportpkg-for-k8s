package jobs

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
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
	dc.K8sCoreClientSet = fake.NewSimpleClientset(objects...)

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
	dc.K8sCoreClientSet = fake.NewSimpleClientset(&corev1.Pod{
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
