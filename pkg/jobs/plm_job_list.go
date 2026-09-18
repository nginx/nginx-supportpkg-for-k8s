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
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/crds"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetPLMNamespaces discovers all PLM namespaces across the cluster and combines them with target namespaces
func GetPLMNamespaces(dc *data_collector.DataCollector, ctx context.Context) []string {
	nsMap := make(map[string]bool)
	for _, ns := range dc.Namespaces {
		if ns != "" {
			nsMap[ns] = true
		}
	}

	if dc.K8sCoreClientSet != nil {
		nsList, err := dc.K8sCoreClientSet.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, ns := range nsList.Items {
				if strings.Contains(ns.Name, "plm") {
					nsMap[ns.Name] = true
					continue
				}
				pods, err := dc.K8sCoreClientSet.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{})
				if err == nil {
					for _, pod := range pods.Items {
						if strings.Contains(pod.Name, "plm") || strings.Contains(pod.Name, "f5-waf-policy-controller") || strings.Contains(pod.Name, "seaweed") {
							nsMap[ns.Name] = true
							break
						}
					}
				}
			}
		} else {
			if _, err := dc.K8sCoreClientSet.CoreV1().Namespaces().Get(ctx, "plm-system", metav1.GetOptions{}); err == nil {
				nsMap["plm-system"] = true
			}
		}
	}

	var plmNamespaces []string
	for ns := range nsMap {
		plmNamespaces = append(plmNamespaces, ns)
	}
	return plmNamespaces
}

// IsPLMNamespace checks if any of the target namespaces or cluster workloads indicate PLM deployment
func IsPLMNamespace(dc *data_collector.DataCollector, ctx context.Context) bool {
	if dc.K8sCoreClientSet == nil {
		for _, namespace := range dc.Namespaces {
			if strings.Contains(namespace, "plm") {
				return true
			}
		}
		return false
	}

	plmNS := GetPLMNamespaces(dc, ctx)
	for _, ns := range plmNS {
		if strings.Contains(ns, "plm") {
			return true
		}
		pods, err := dc.K8sCoreClientSet.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, pod := range pods.Items {
				if strings.Contains(pod.Name, "plm") || strings.Contains(pod.Name, "f5-waf-policy-controller") || strings.Contains(pod.Name, "seaweed") {
					return true
				}
			}
		}
	}
	return false
}

func PLMJobList() []Job {
	jobList := []Job{
		{
			Name:    "plm-crd-objects",
			Timeout: time.Second * 10,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				plmNamespaces := GetPLMNamespaces(dc, ctx)

				for _, namespace := range plmNamespaces {
					for _, crd := range crds.GetPLMCRDList() {
						crdFilePath := filepath.Join(dc.BaseDir, "crds", namespace, crd.Resource+".json")
						if _, err := os.Stat(crdFilePath); err == nil {
							// Already collected for this namespace by a prior job (e.g. NIC crd-objects)
							continue
						}
						result, err := dc.QueryCRD(crd, namespace, ctx)
						if err != nil {
							dc.Logger.Printf("\tCRD %s.%s/%s could not be collected in namespace %s: %v\n", crd.Resource, crd.Group, crd.Version, namespace, err)
						} else {
							var jsonResult bytes.Buffer
							_ = json.Indent(&jsonResult, result, "", "  ")
							jobResult.Files[crdFilePath] = jsonResult.Bytes()
						}
					}
				}
				ch <- jobResult
			},
		},
		{
			Name:    "plm-pod-logs",
			Timeout: time.Second * 30,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				plmNamespaces := GetPLMNamespaces(dc, ctx)

				for _, namespace := range plmNamespaces {
					if dc.K8sCoreClientSet == nil {
						continue
					}
					pods, err := dc.K8sCoreClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						dc.Logger.Printf("\tCould not retrieve pod list for namespace %s: %v\n", namespace, err)
						continue
					}
					for _, pod := range pods.Items {
						if strings.Contains(pod.Name, "plm") || strings.Contains(pod.Name, "policy-controller") || strings.Contains(pod.Name, "seaweed") {
							for _, container := range pod.Spec.Containers {
								podLogOptions := v1.PodLogOptions{
									Container: container.Name,
								}
								req := dc.K8sCoreClientSet.CoreV1().Pods(namespace).GetLogs(pod.Name, &podLogOptions)
								podLogs, err := req.Stream(ctx)
								if err != nil {
									dc.Logger.Printf("\tCould not retrieve logs for container %s in pod %s namespace %s: %v\n", container.Name, pod.Name, namespace, err)
									continue
								}
								buf := new(bytes.Buffer)
								_, err = io.Copy(buf, podLogs)
								_ = podLogs.Close()
								if err != nil {
									dc.Logger.Printf("\tCould not read logs for container %s in pod %s namespace %s: %v\n", container.Name, pod.Name, namespace, err)
									continue
								}
								jobResult.Files[filepath.Join(dc.BaseDir, "logs", namespace, pod.Name+"__"+container.Name+".log")] = buf.Bytes()
							}
						}
					}
				}
				ch <- jobResult
			},
		},
		{
			Name:    "plm-storage-info",
			Timeout: time.Second * 10,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				plmNamespaces := GetPLMNamespaces(dc, ctx)

				for _, namespace := range plmNamespaces {
					if dc.K8sCoreClientSet == nil {
						continue
					}
					pvcs, err := dc.K8sCoreClientSet.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
					if err == nil {
						jsonPVCs, _ := json.MarshalIndent(pvcs, "", "  ")
						jobResult.Files[filepath.Join(dc.BaseDir, "storage", namespace, "pvc-list.json")] = jsonPVCs
					}
				}
				ch <- jobResult
			},
		},
		{
			Name:    "plm-entitlement-secret",
			Timeout: time.Second * 10,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				plmNamespaces := GetPLMNamespaces(dc, ctx)

				for _, namespace := range plmNamespaces {
					if dc.K8sCoreClientSet == nil {
						continue
					}
					secrets, err := dc.K8sCoreClientSet.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						continue
					}
					for _, secret := range secrets.Items {
						if licenseToken, exists := secret.Data["license.jwt"]; exists {
							parts := strings.Split(string(licenseToken), ".")
							if len(parts) >= 2 {
								decodedClaim, err := base64.RawStdEncoding.DecodeString(parts[1])
								if err == nil {
									var prettyJSON bytes.Buffer
									if json.Indent(&prettyJSON, decodedClaim, "", "  ") == nil {
										jobResult.Files[filepath.Join(dc.BaseDir, "entitlement", namespace, secret.Name+"_payload.json")] = prettyJSON.Bytes()
									}
								}
							}
						}
					}
				}
				ch <- jobResult
			},
		},
	}
	return jobList
}
