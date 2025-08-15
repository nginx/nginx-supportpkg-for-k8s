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
	"fmt"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/crds"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path/filepath"
	"strings"
	"time"
)

func NICJobList() []Job {
	jobList := []Job{
		{
			Name:    "exec-nginx-ingress-version",
			Timeout: time.Second * 10,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				command := []string{"./nginx-ingress", "--version"}
				for _, namespace := range dc.Namespaces {
					pods, err := dc.K8sCoreClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						dc.Logger.Printf("\tCould not retrieve pod list for namespace %s: %v\n", namespace, err)
					} else {
						for _, pod := range pods.Items {
							if strings.Contains(pod.Name, "ingress") {
								for _, container := range pod.Spec.Containers {

									res, err := dc.PodExecutor(namespace, pod.Name, container.Name, command, ctx)
									if err != nil {
										jobResult.Error = err
										dc.Logger.Printf("\tCommand execution %s failed for pod %s in namespace %s: %v\n", command, pod.Name, namespace, err)
									} else {
										fileName := fmt.Sprintf("%s__%s__nginx-ingress-version.txt", pod.Name, container.Name)
										jobResult.Files[filepath.Join(dc.BaseDir, "exec", namespace, fileName)] = res
									}
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
							if strings.Contains(pod.Name, "ingress") {
								for _, container := range pod.Spec.Containers {
									res, err := dc.PodExecutor(namespace, pod.Name, container.Name, command, ctx)
									if err != nil {
										jobResult.Error = err
										dc.Logger.Printf("\tCommand execution %s failed for pod %s in namespace %s: %v\n", command, pod.Name, namespace, err)
									} else {
										fileName := fmt.Sprintf("%s__%s__nginx-t.txt", pod.Name, container.Name)
										jobResult.Files[filepath.Join(dc.BaseDir, "exec", namespace, fileName)] = res
									}
								}
							}
						}
					}
				}
				ch <- jobResult
			},
		},
		{
			Name:    "exec-agent-conf",
			Timeout: time.Second * 10,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				command := []string{"cat", "/etc/nginx-agent/nginx-agent.conf"}
				for _, namespace := range dc.Namespaces {
					pods, err := dc.K8sCoreClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						dc.Logger.Printf("\tCould not retrieve pod list for namespace %s: %v\n", namespace, err)
					} else {
						for _, pod := range pods.Items {
							if strings.Contains(pod.Name, "ingress") {
								for _, container := range pod.Spec.Containers {
									res, err := dc.PodExecutor(namespace, pod.Name, container.Name, command, ctx)
									if err != nil {
										jobResult.Error = err
										dc.Logger.Printf("\tCommand execution %s failed for pod %s in namespace %s: %v\n", command, pod.Name, namespace, err)
									} else {
										fileName := fmt.Sprintf("%s__%s__nginx-agent.conf", pod.Name, container.Name)
										jobResult.Files[filepath.Join(dc.BaseDir, "exec", namespace, fileName)] = res
									}
								}
							}
						}
					}
				}
				ch <- jobResult
			},
		},
		{
			Name:    "exec-agent-version",
			Timeout: time.Second * 10,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				command := []string{"/usr/bin/nginx-agent", "--version"}
				for _, namespace := range dc.Namespaces {
					pods, err := dc.K8sCoreClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						dc.Logger.Printf("\tCould not retrieve pod list for namespace %s: %v\n", namespace, err)
					} else {
						for _, pod := range pods.Items {
							if strings.Contains(pod.Name, "ingress") {
								for _, container := range pod.Spec.Containers {
									res, err := dc.PodExecutor(namespace, pod.Name, container.Name, command, ctx)
									if err != nil {
										jobResult.Error = err
										dc.Logger.Printf("\tCommand execution %s failed for pod %s in namespace %s: %v\n", command, pod.Name, namespace, err)
									} else {
										fileName := fmt.Sprintf("%s__%s__nginx-agent-version.txt", pod.Name, container.Name)
										jobResult.Files[filepath.Join(dc.BaseDir, "exec", namespace, fileName)] = res
									}
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
					for _, crd := range crds.GetNICCRDList() {
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
