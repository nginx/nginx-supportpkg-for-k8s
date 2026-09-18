package jobs

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"path/filepath"
	"testing"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/crds"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPLMJobList(t *testing.T) {
	jobs := PLMJobList()
	assert.NotEmpty(t, jobs, "expected jobs to be returned")

	expectedJobs := []string{"plm-crd-objects", "plm-pod-logs", "plm-storage-info", "plm-entitlement-secret"}
	assert.Len(t, jobs, len(expectedJobs))

	for i, job := range jobs {
		assert.Equal(t, expectedJobs[i], job.Name)
		assert.NotNil(t, job.Execute)
		assert.NotZero(t, job.Timeout)
	}
}

func TestIsPLMNamespace(t *testing.T) {
	ctx := context.Background()

	// Case 1: plm in namespace name
	dc1 := &data_collector.DataCollector{
		Namespaces:       []string{"plm-system"},
		K8sCoreClientSet: fake.NewClientset(),
	}
	assert.True(t, IsPLMNamespace(dc1, ctx))

	// Case 2: plm pod in default namespace
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "plm-f5-waf-policy-controller-123",
			Namespace: "default",
		},
	}
	dc2 := &data_collector.DataCollector{
		Namespaces:       []string{"default"},
		K8sCoreClientSet: fake.NewClientset(pod),
	}
	assert.True(t, IsPLMNamespace(dc2, ctx))

	// Case 3: no plm resources
	dc3 := &data_collector.DataCollector{
		Namespaces:       []string{"default"},
		K8sCoreClientSet: fake.NewClientset(),
	}
	assert.False(t, IsPLMNamespace(dc3, ctx))
}

func TestPLMJobList_CRDObjects(t *testing.T) {
	tmpDir := t.TempDir()
	dc := &data_collector.DataCollector{
		BaseDir:    tmpDir,
		Namespaces: []string{"plm-system"},
		Logger:     log.New(io.Discard, "", 0),
		QueryCRD: func(crd crds.Crd, namespace string, ctx context.Context) ([]byte, error) {
			mockData := map[string]interface{}{
				"apiVersion": crd.Group + "/" + crd.Version,
				"kind":       crd.Resource,
				"items":      []interface{}{},
			}
			return json.Marshal(mockData)
		},
	}

	jobs := PLMJobList()
	crdJob := jobs[0]
	assert.Equal(t, "plm-crd-objects", crdJob.Name)

	ctx := context.Background()
	ch := make(chan JobResult, 1)
	crdJob.Execute(dc, ctx, ch)

	result := <-ch
	assert.Nil(t, result.Error)
	assert.Len(t, result.Files, len(crds.GetPLMCRDList()))
}

func TestPLMJobList_EntitlementSecret(t *testing.T) {
	tmpDir := t.TempDir()
	// Dummy JWT header.payload.sig with base64 encoded JSON {"sub":"test-license"}
	dummyJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LWxpY2Vuc2UifQ.signature"

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jwt-reg-secret",
			Namespace: "plm-system",
		},
		Data: map[string][]byte{
			"license.jwt": []byte(dummyJWT),
		},
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "plm-system",
		},
	}

	client := fake.NewClientset(secret, ns)
	dc := &data_collector.DataCollector{
		BaseDir:          tmpDir,
		Namespaces:       []string{"plm-system"},
		Logger:           log.New(io.Discard, "", 0),
		K8sCoreClientSet: client,
	}

	jobs := PLMJobList()
	entitlementJob := jobs[3]
	assert.Equal(t, "plm-entitlement-secret", entitlementJob.Name)

	ctx := context.Background()
	ch := make(chan JobResult, 1)
	entitlementJob.Execute(dc, ctx, ch)

	result := <-ch
	assert.Nil(t, result.Error)
	assert.NotEmpty(t, result.Files)

	expectedPath := filepath.Join(tmpDir, "entitlement", "plm-system", "jwt-reg-secret_payload.json")
	content, exists := result.Files[expectedPath]
	assert.True(t, exists)
	assert.Contains(t, string(content), "test-license")
}
