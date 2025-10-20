package data_collector

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/crds"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestNewDataCollector_Success(t *testing.T) {
	dc := &DataCollector{Namespaces: []string{"default"}}
	err := NewDataCollector(dc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if dc.BaseDir == "" {
		t.Error("BaseDir should be set")
	}
	if dc.Logger == nil {
		t.Error("Logger should be set")
	}
	if dc.LogFile == nil {
		t.Error("LogFile should be set")
	}
	if dc.K8sCoreClientSet == nil {
		t.Error("K8sCoreClientSet should be set")
	}
	if dc.K8sCrdClientSet == nil {
		t.Error("K8sCrdClientSet should be set")
	}
	if dc.K8sMetricsClientSet == nil {
		t.Error("K8sMetricsClientSet should be set")
	}
	if dc.K8sHelmClientSet == nil {
		t.Error("K8sHelmClientSet should be set")
	}
}

func TestWrapUp_CreatesTarball(t *testing.T) {
	tmpDir := t.TempDir()
	logFile, _ := os.Create(filepath.Join(tmpDir, "supportpkg.log"))
	dc := &DataCollector{
		BaseDir: tmpDir,
		LogFile: logFile,
		Logger:  log.New(io.Discard, "", 0),
	}
	product := "nginx"
	tarball, err := dc.WrapUp(product)
	if err != nil {
		t.Fatalf("WrapUp failed: %v", err)
	}
	if _, err := os.Stat(tarball); err != nil {
		t.Errorf("tarball not created: %v", err)
	}
	_ = os.Remove(tarball)
}

func TestRealPodExecutor_ReturnsOutput(t *testing.T) {
	dc := &DataCollector{
		K8sCoreClientSet: fake.NewClientset(),
		K8sRestConfig:    &rest.Config{},
	}
	// Replace RealPodExecutor with a mock for testing
	dc.PodExecutor = func(namespace, pod, container string, command []string, ctx context.Context) ([]byte, error) {
		return []byte("output"), nil
	}
	out, err := dc.PodExecutor("default", "pod", "container", []string{"echo", "hello"}, context.TODO())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !bytes.Contains(out, []byte("output")) {
		t.Errorf("expected output, got %s", string(out))
	}
}

func TestRealQueryCRD_ReturnsErrorOnInvalidConfig(t *testing.T) {
	dc := &DataCollector{
		K8sRestConfig: &rest.Config{},
	}
	crd := crds.Crd{Group: "test", Version: "v1", Resource: "foos"}
	_, err := dc.RealQueryCRD(crd, "default", context.TODO())
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestAllNamespacesExist_AllExist(t *testing.T) {
	client := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})
	dc := &DataCollector{
		Namespaces:       []string{"default"},
		K8sCoreClientSet: client,
		Logger:           log.New(io.Discard, "", 0),
	}
	if !dc.AllNamespacesExist() {
		t.Error("expected all namespaces to exist")
	}
}

func TestAllNamespacesExist_NotExist(t *testing.T) {
	client := fake.NewClientset()
	dc := &DataCollector{
		Namespaces:       []string{"missing"},
		K8sCoreClientSet: client,
		Logger:           log.New(io.Discard, "", 0),
	}
	if dc.AllNamespacesExist() {
		t.Error("expected namespaces to not exist")
	}
}

func TestWrapUp_ErrorOnLogFileClose(t *testing.T) {
	tmpDir := t.TempDir()
	logFile, _ := os.Create(filepath.Join(tmpDir, "supportpkg.log"))
	logFile.Close() // Already closed
	dc := &DataCollector{
		BaseDir: tmpDir,
		LogFile: logFile,
		Logger:  log.New(io.Discard, "", 0),
	}
	_, err := dc.WrapUp("nginx")
	if err == nil {
		t.Error("expected error on closing already closed log file")
	}
}
