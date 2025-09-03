package jobs

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/kubernetes/fake"
)

// helper creates int32 ptr
func i32(v int32) *int32 { return &v }

func setupDataCollector(t *testing.T) *data_collector.DataCollector {
	t.Helper()

	tmpDir := t.TempDir()

	// Seed fake objects (namespace default implied by metadata)
	objs := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c1", Image: "nginx:latest"}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-1", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "demo"}},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dep-1", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: i32(1),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "demo"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "dep-c1", Image: "nginx:latest"}},
					},
				},
			},
		},
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "role-1", Namespace: "default"},
			Rules:      []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cm-1", Namespace: "default"},
			Data:       map[string]string{"k": "v"},
		},
	}

	client := fake.NewSimpleClientset(objs...)

	return &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"default"},
		Logger:           log.New(io.Discard, "", 0),
		K8sCoreClientSet: client,
		// Leave other client sets nil; we will not execute jobs that depend on them in this focused test.
	}
}

func TestCommonJobList_SelectedJobsProduceFiles(t *testing.T) {
	dc := setupDataCollector(t)

	// Jobs we explicitly validate (keep focused; others require additional fake clients)
	targetJobs := map[string]struct{}{
		"pod-list":        {},
		"service-list":    {},
		"deployment-list": {},
		"roles-list":      {},
		"configmap-list":  {},
	}

	jobList := CommonJobList()

	for _, job := range jobList {
		if _, ok := targetJobs[job.Name]; !ok {
			continue
		}

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
				if !filepath.HasPrefix(path, dc.BaseDir) { // acceptable here; test code (Go <1.22 deprecation not critical)
					t.Fatalf("job %s file path %s does not start with basedir %s", job.Name, path, dc.BaseDir)
				}
			}
		case <-ctx.Done():
			t.Fatalf("job %s timed out", job.Name)
		}
	}
}

func TestCommonJobList_PodListJSONKeyPresence(t *testing.T) {
	dc := setupDataCollector(t)
	var podListJob *Job
	for i, j := range CommonJobList() {
		if j.Name == "pod-list" {
			podListJob = &CommonJobList()[i]
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
