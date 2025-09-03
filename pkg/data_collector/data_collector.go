/**

Copyright 2024 F5, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

**/

package data_collector

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	helmClient "github.com/mittwald/go-helm-client"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/crds"
	corev1 "k8s.io/api/core/v1"
	crdClient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
	metricsClient "k8s.io/metrics/pkg/client/clientset/versioned"
)

type DataCollector struct {
	BaseDir               string
	Namespaces            []string
	Logger                *log.Logger
	LogFile               *os.File
	K8sRestConfig         *rest.Config
	K8sCoreClientSet      kubernetes.Interface //*kubernetes.Clientset
	K8sCrdClientSet       *crdClient.Clientset
	K8sMetricsClientSet   *metricsClient.Clientset
	K8sHelmClientSet      map[string]helmClient.Client
	ExcludeDBData         bool
	ExcludeTimeSeriesData bool
	PodExecutor           func(namespace, pod, container string, command []string, ctx context.Context) ([]byte, error)
	QueryCRD              func(crd crds.Crd, namespace string, ctx context.Context) ([]byte, error)
}

func NewDataCollector(collector *DataCollector) error {

	tmpDir, err := os.MkdirTemp("", "-pkg-diag")
	if err != nil {
		return fmt.Errorf("unable to create temp directory: %s", err)
	}

	logFile, err := os.OpenFile(filepath.Join(tmpDir, "supportpkg.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("unable to create log file: %s", err)
	}

	// Find config
	kubeConfig := os.Getenv("KUBECONFIG")
	if kubeConfig == "" {
		kubeConfig = filepath.Join(homedir.HomeDir(), ".kube", "config")
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)

	if err != nil {
		return fmt.Errorf("unable to connect to k8s using file %s: %s", kubeConfig, err)
	}
	// Set up the DataCollector options
	collector.BaseDir = tmpDir
	collector.LogFile = logFile
	collector.Logger = log.New(logFile, "", log.LstdFlags|log.LUTC|log.Lmicroseconds|log.Lshortfile)
	collector.K8sHelmClientSet = make(map[string]helmClient.Client)
	collector.PodExecutor = collector.RealPodExecutor
	collector.QueryCRD = collector.RealQueryCRD

	//Initialize clients
	collector.K8sRestConfig = config
	collector.K8sCoreClientSet, _ = kubernetes.NewForConfig(config)
	collector.K8sCrdClientSet, _ = crdClient.NewForConfig(config)
	collector.K8sMetricsClientSet, _ = metricsClient.NewForConfig(config)
	for _, namespace := range collector.Namespaces {
		collector.K8sHelmClientSet[namespace], _ = helmClient.NewClientFromRestConf(&helmClient.RestConfClientOptions{
			Options:    &helmClient.Options{Namespace: namespace},
			RestConfig: config,
		})
	}

	return nil
}

func (c *DataCollector) WrapUp(product string) (string, error) {

	unixTime := time.Now().Unix()
	unixTimeString := strconv.FormatInt(unixTime, 10)
	tarballName := fmt.Sprintf("%s-supportpkg-%s.tar.gz", product, unixTimeString)
	tarballRootDirName := fmt.Sprintf("%s-supportpkg-%s", product, unixTimeString)

	err := c.LogFile.Close()
	if err != nil {
		return tarballName, err
	}

	file, err := os.Create(tarballName)
	if err != nil {
		return tarballName, err
	}
	defer func(file *os.File) {
		cerr := file.Close()
		if cerr == nil {
			err = cerr
		} else {
			c.Logger.Printf("error closing file %s, %v", file.Name(), cerr)
		}
	}(file)

	gw := gzip.NewWriter(file)
	defer func(gw *gzip.Writer) {
		cerr := gw.Close()
		if cerr == nil {
			err = cerr
		} else {
			c.Logger.Printf("error closing gzip writer, %v", cerr)
		}
	}(gw)

	tw := tar.NewWriter(gw)
	defer func(tw *tar.Writer) {
		cerr := tw.Close()
		if cerr == nil {
			err = cerr
		} else {
			c.Logger.Printf("error closing tar writer, %v", cerr)
		}
	}(tw)

	err = filepath.Walk(c.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(c.BaseDir, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(tarballRootDirName, relativePath))

		if err = tw.WriteHeader(header); err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		file, err = os.Open(path)
		if err != nil {
			return err
		}
		defer func(file *os.File) {
			cerr := file.Close()
			if cerr == nil {
				err = cerr
			} else {
				c.Logger.Printf("error closing file %s, %v", file.Name(), cerr)
			}
		}(file)

		_, err = io.Copy(tw, file)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return tarballName, err
	}
	_ = os.RemoveAll(c.BaseDir)
	return tarballName, nil
}

func (c *DataCollector) RealPodExecutor(namespace string, pod string, container string, command []string, ctx context.Context) ([]byte, error) {
	req := c.K8sCoreClientSet.CoreV1().RESTClient().Post().
		Namespace(namespace).
		Resource("pods").
		Name(pod).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command:   command,
			Container: container,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.K8sRestConfig, "POST", req.URL())
	if err != nil {
		return nil, err
	}
	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if stdout.Len() > 0 || stderr.Len() > 0 {
		response := append(stdout.Bytes(), stderr.Bytes()...)
		return response, nil
	} else {
		return nil, err
	}
}

func (c *DataCollector) RealQueryCRD(crd crds.Crd, namespace string, ctx context.Context) ([]byte, error) {

	schemeGroupVersion := schema.GroupVersion{Group: crd.Group, Version: crd.Version}
	negotiatedSerializer := scheme.Codecs.WithoutConversion()
	c.K8sRestConfig.APIPath = "apis"
	c.K8sRestConfig.GroupVersion = &schemeGroupVersion
	c.K8sRestConfig.NegotiatedSerializer = negotiatedSerializer

	client, err := rest.RESTClientFor(c.K8sRestConfig)
	if err != nil {
		return nil, err
	}

	result := client.Get().
		Namespace(namespace). // Specify the namespace if needed
		Resource(crd.Resource).
		Do(ctx)

	return result.Raw()
}

func (c *DataCollector) AllNamespacesExist() bool {
	var allExist = true
	for _, namespace := range c.Namespaces {
		_, err := c.K8sCoreClientSet.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
		if err != nil {
			c.Logger.Printf("\t%s: %v\n", namespace, err)
			fmt.Printf("\t%s: %v\n", namespace, err)
			allExist = false
		}
	}

	return allExist
}
