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

package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/crds"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path/filepath"
	"strings"
	"time"
)

func NGFJobList() []Job {
	jobList := []Job{
		{
			Name:    "exec-nginx-gateway-version",
			Timeout: time.Second * 10,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				command := []string{"/usr/bin/gateway", "--help"}
				for _, namespace := range dc.Namespaces {
					pods, err := dc.K8sCoreClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						dc.Logger.Printf("\tCould not retrieve pod list for namespace %s: %v\n", namespace, err)
					} else {
						for _, pod := range pods.Items {
							if strings.Contains(pod.Name, "nginx-gateway") {
								res, err := dc.PodExecutor(namespace, pod.Name, "nginx-gateway", command, ctx)
								if err != nil {
									jobResult.Error = err
									dc.Logger.Printf("\tCommand execution %s failed for pod %s in namespace %s: %v\n", command, pod.Name, namespace, err)
								} else {
									jobResult.Files[filepath.Join(dc.BaseDir, "exec", namespace, pod.Name+"__nginx-gateway-version.txt")] = res
								}
							}
						}
					}
				}
				ch <- jobResult
			},
		},
		{
			Name:    "exec-nginx-t",
			Timeout: time.Second * 10,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				command := []string{"/usr/sbin/nginx", "-T"}
				for _, namespace := range dc.Namespaces {
					pods, err := dc.K8sCoreClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						dc.Logger.Printf("\tCould not retrieve pod list for namespace %s: %v\n", namespace, err)
					} else {
						for _, pod := range pods.Items {
							if strings.Contains(pod.Name, "nginx-gateway") {
								res, err := dc.PodExecutor(namespace, pod.Name, "nginx", command, ctx)
								if err != nil {
									jobResult.Error = err
									dc.Logger.Printf("\tCommand execution %s failed for pod %s in namespace %s: %v\n", command, pod.Name, namespace, err)
								} else {
									jobResult.Files[filepath.Join(dc.BaseDir, "exec", namespace, pod.Name+"__nginx-t.txt")] = res
								}
							}
						}
					}
				}
				ch <- jobResult
			},
		},
		{
			Name:    "crd-objects",
			Timeout: time.Second * 10,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				for _, namespace := range dc.Namespaces {
					for _, crd := range crds.GetNGFCRDList() {
						result, err := dc.QueryCRD(crd, namespace, ctx)
						if err != nil {
							dc.Logger.Printf("\tCRD %s.%s/%s could not be collected in namespace %s: %v\n", crd.Resource, crd.Group, crd.Version, namespace, err)
						} else {
							var jsonResult bytes.Buffer
							_ = json.Indent(&jsonResult, result, "", "  ")
							jobResult.Files[filepath.Join(dc.BaseDir, "crds", namespace, crd.Resource+".json")] = jsonResult.Bytes()
						}
					}
				}
				ch <- jobResult
			},
		},
	}
	return jobList
}
