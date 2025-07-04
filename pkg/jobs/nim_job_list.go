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
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var dqliteBackupPath = "/etc/nms/scripts/dqlite-backup"
var nmsConfigPath = "/etc/nms/nms.conf"

var clickhouseCommands = map[string]string{
	"events.csv":                        "SELECT * FROM nms.events WHERE creation_time > subtractHours(now(),${default_hrs}) ORDER BY creation_time DESC LIMIT ${max_log_limit} FORMAT CSVWithNames",
	"metrics.csv":                       "SELECT * FROM nms.metrics WHERE timestamp > subtractHours(now(),${default_hrs}) AND date > toDate(subtractDays(now(),${max_num_days})) ORDER BY timestamp DESC LIMIT ${max_log_limit} FORMAT CSVWithNames",
	"metrics_1day.csv":                  "SELECT * FROM nms.metrics_1day WHERE timestamp > subtractHours(now(),${default_hrs}) AND date > toDate(subtractDays(now(),${max_num_days})) ORDER BY timestamp DESC LIMIT ${max_log_limit} FORMAT CSVWithNames",
	"metrics_1hour.csv":                 "SELECT * FROM nms.metrics_1hour WHERE timestamp > subtractHours(now(),${default_hrs}) AND date > toDate(subtractDays(now(),${max_num_days})) ORDER BY timestamp DESC LIMIT ${max_log_limit} FORMAT CSVWithNames",
	"metrics_5min.csv":                  "SELECT * FROM nms.metrics_5min WHERE timestamp > subtractHours(now(),${default_hrs}) AND date > toDate(subtractDays(now(),${max_num_days})) ORDER BY timestamp DESC LIMIT ${max_log_limit} FORMAT CSVWithNames",
	"metrics-row-counts.csv":            "SELECT count(*), name FROM nms.metrics GROUP BY name FORMAT CSVWithNames",
	"events-row-counts.csv":             "SELECT count(*), category FROM nms.events GROUP BY category FORMAT CSVWithNames",
	"events.sql":                        "SHOW CREATE TABLE nms.events",
	"metrics.sql":                       "SHOW CREATE TABLE nms.metrics",
	"metrics_1day.sql":                  "SHOW CREATE TABLE nms.metrics_1day",
	"metrics_1hour.sql":                 "SHOW CREATE TABLE nms.metrics_1hour",
	"metrics_5min.sql":                  "SHOW CREATE TABLE nms.metrics_5min",
	"table-sizes.stat":                  "SELECT table, formatReadableSize(sum(bytes)) as size, min(min_date) as min_date, max(max_date) as max_date FROM system.parts WHERE active GROUP BY table ORDER BY table FORMAT CSVWithNames",
	"system-asynchronous-metrics.stat":  "SELECT * FROM system.asynchronous_metrics FORMAT CSVWithNames",
	"system-tables.stat":                "SELECT * FROM system.tables FORMAT CSVWithNames",
	"system-parts.stat":                 "SELECT * FROM system.parts ORDER BY table ASC, name DESC, modification_time DESC FORMAT CSVWithNames",
	"system-metrics.stat":               "SELECT * FROM system.metrics FORMAT CSVWithNames",
	"system-settings.stat":              "SELECT * FROM system.settings FORMAT CSVWithNames",
	"system-query-log.csv":              "SELECT * FROM system.query_log WHERE event_time > subtractHours(now(),${default_hrs}) AND event_date > toDate(subtractDays(now(),${max_num_days})) ORDER BY event_time DESC LIMIT ${max_log_limit} FORMAT CSVWithNames",
	"system-events.stat":                "SELECT * FROM system.events ORDER BY value DESC LIMIT ${max_log_limit} FORMAT CSVWithNames",
	"metrics-profile-15-min.csv":        "SELECT hex(SHA256(instance)) inst, name, toStartOfInterval(timestamp, INTERVAL 15 minute) tmstp, count(*) cnt FROM metrics where timestamp > date_add(month, -1, timestamp) GROUP BY tmstp, inst, name ORDER BY inst, name, tmstp asc LIMIT 1000000;",
	"metrics-profile-1-hour.csv":        "SELECT hex(SHA256(instance)) inst, name, toStartOfInterval(timestamp, INTERVAL 1 hour) tmstp, count(*) cnt FROM metrics where timestamp > date_add(month, -1, timestamp) GROUP BY tmstp, inst, name ORDER BY inst, name, tmstp asc LIMIT 1000000;",
	"metrics-profile-1-day.csv":         "SELECT hex(SHA256(instance)) inst, name, toStartOfInterval(timestamp, INTERVAL 1 day) tmstp, count(*) cnt FROM metrics where timestamp > date_add(month, -1, timestamp) GROUP BY tmstp, inst, name ORDER BY inst, name, tmstp asc LIMIT 1000000;",
	"metrics_5min-profile-1-hour.csv":   "SELECT hex(SHA256(instance)) inst, name, toStartOfInterval(timestamp, INTERVAL 1 hour) tmstp, count(*) cnt FROM metrics_5min where timestamp > date_add(month, -1, timestamp) GROUP BY tmstp, inst, name ORDER BY inst, name, tmstp asc LIMIT 1000000;",
	"metrics_5min-profile-1-day.csv":    "SELECT hex(SHA256(instance)) inst, name, toStartOfInterval(timestamp, INTERVAL 1 day) tmstp, count(*) cnt FROM metrics_5min where timestamp > date_add(month, -1, timestamp) GROUP BY tmstp, inst, name ORDER BY inst, name, tmstp asc LIMIT 1000000;",
	"metrics_1hour-profile-1-day.csv":   "SELECT hex(SHA256(instance)) inst, name, toStartOfInterval(timestamp, INTERVAL 1 day) tmstp, count(*) cnt FROM metrics_1hour where timestamp > date_add(month, -1, timestamp) GROUP BY tmstp, inst, name ORDER BY inst, name, tmstp asc LIMIT 1000000;",
	"metrics_1hour-profile-1-month.csv": "SELECT hex(SHA256(instance)) inst, name, toStartOfInterval(timestamp, INTERVAL 1 month) tmstp, count(*) cnt FROM metrics_1hour where timestamp > date_add(month, -1, timestamp) GROUP BY tmstp, inst, name ORDER BY inst, name, tmstp asc LIMIT 1000000;",
	"metrics_1day-profile-1-day.csv":    "SELECT hex(SHA256(instance)) inst, name, toStartOfInterval(timestamp, INTERVAL 1 day) tmstp, count(*) cnt FROM metrics_1day where timestamp > date_add(month, -1, timestamp) GROUP BY tmstp, inst, name ORDER BY inst, name, tmstp asc LIMIT 1000000;",
	"metrics_1day-profile-1-month.csv":  "SELECT hex(SHA256(instance)) inst, name, toStartOfInterval(timestamp, INTERVAL 1 month) tmstp, count(*) cnt FROM metrics_1day where timestamp > date_add(month, -1, timestamp) GROUP BY tmstp, inst, name ORDER BY inst, name, tmstp asc LIMIT 1000000;",
}

func NIMJobList() []Job {
	jobList := []Job{
		{
			Name:    "exec-apigw-nginx-t",
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
							if strings.Contains(pod.Name, "apigw") {
								res, err := dc.PodExecutor(namespace, pod.Name, "apigw", command, ctx)
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
			Name:    "exec-apigw-nginx-version",
			Timeout: time.Second * 10,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				command := []string{"/usr/sbin/nginx", "-v"}
				for _, namespace := range dc.Namespaces {
					pods, err := dc.K8sCoreClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						dc.Logger.Printf("\tCould not retrieve pod list for namespace %s: %v\n", namespace, err)
					} else {
						for _, pod := range pods.Items {
							if strings.Contains(pod.Name, "apigw") {
								res, err := dc.PodExecutor(namespace, pod.Name, "apigw", command, ctx)
								if err != nil {
									jobResult.Error = err
									dc.Logger.Printf("\tCommand execution %s failed for pod %s in namespace %s: %v\n", command, pod.Name, namespace, err)
								} else {
									jobResult.Files[filepath.Join(dc.BaseDir, "exec", namespace, pod.Name+"__nginx-version.txt")] = res
								}
							}
						}
					}
				}
				ch <- jobResult
			},
		},
		{
			Name:    "exec-clickhouse-version",
			Timeout: time.Second * 10,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				command := []string{"clickhouse-server", "--version"}
				for _, namespace := range dc.Namespaces {
					pods, err := dc.K8sCoreClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						dc.Logger.Printf("\tCould not retrieve pod list for namespace %s: %v\n", namespace, err)
					} else {
						for _, pod := range pods.Items {
							if strings.Contains(pod.Name, "clickhouse") {
								res, err := dc.PodExecutor(namespace, pod.Name, "clickhouse-server", command, ctx)
								if err != nil {
									jobResult.Error = err
									dc.Logger.Printf("\tCommand execution %s failed for pod %s in namespace %s: %v\n", command, pod.Name, namespace, err)
								} else {
									jobResult.Files[filepath.Join(dc.BaseDir, "exec", namespace, pod.Name+"__clickhouse-server-version.txt")] = res
								}
							}
						}
					}
				}
				ch <- jobResult
			},
		},
		{
			Name:    "exec-clickhouse-data",
			Timeout: time.Second * 100,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				// command := []string{"clickhouse-client", "--database", "nms", "-q", "SHOW CREATE TABLE nms.events"}
				for _, namespace := range dc.Namespaces {
					pods, err := dc.K8sCoreClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						dc.Logger.Printf("\tCould not retrieve pod list for namespace %s: %v\n", namespace, err)
					} else {
						for _, pod := range pods.Items {
							if strings.Contains(pod.Name, "clickhouse") {
								for fileName, query := range clickhouseCommands {
									// Replace placeholders in the query
									query = strings.ReplaceAll(query, "${default_hrs}", "24")
									query = strings.ReplaceAll(query, "${max_num_days}", "30")
									query = strings.ReplaceAll(query, "${max_log_limit}", "1000")
									command := []string{"clickhouse-client", "--database", "nms", "-q", query}
									if fileName == "events.csv" || fileName == "metrics.csv" {
										command = append(command, "--format_csv_delimiter=,")
									}

									// Execute the command
									res, err := dc.PodExecutor(namespace, pod.Name, "clickhouse-server", command, ctx)
									if err != nil {
										jobResult.Error = err
										dc.Logger.Printf("\tCommand execution %s failed for pod %s in namespace %s: %v\n", command, pod.Name, namespace, err)
									} else {
										jobResult.Files[filepath.Join(dc.BaseDir, "exec", namespace, pod.Name+"__"+fileName)] = res
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
			Name:    "exec-dqlite-dump-core",
			Timeout: time.Second * 30,
			Execute: func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult) {
				jobResult := JobResult{Files: make(map[string][]byte), Error: nil}
				containerName := "core"
				dbName := "core"
				outputFile := "/tmp/core.sql"
				dbAddr := "0.0.0.0:7891"

				// /etc/nms/scripts/dqlite-backup -n core -c /etc/nms/nms.conf -a 0.0.0.0:7891 -o /tmp/core.sql -k
				command := []string{dqliteBackupPath, "-n", dbName, "-c", nmsConfigPath, "-a", dbAddr, "-o", outputFile, "-k"}
				for _, namespace := range dc.Namespaces {
					pods, err := dc.K8sCoreClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						dc.Logger.Printf("\tCould not retrieve pod list for namespace %s: %v\n", namespace, err)
					} else {
						for _, pod := range pods.Items {
							if strings.Contains(pod.Name, containerName) {
								res, err := dc.PodExecutor(namespace, pod.Name, containerName, command, ctx)
								if err != nil {
									jobResult.Error = err
									dc.Logger.Printf("\tCommand execution %s failed for pod %s in namespace %s: %v\n", command, pod.Name, namespace, err)
								} else {
									jobResult.Files[filepath.Join(dc.BaseDir, "exec", namespace, pod.Name+"__dqlite-dump-"+containerName+".txt")] = res

									// Move the dumped file to the base directory
									destPathFilename := filepath.Join(dc.BaseDir, "exec", namespace, pod.Name+"__dqlite-dump-"+filepath.Base(outputFile))
									if err := dc.CopyFileFromPod(namespace, pod.Name, containerName, outputFile, destPathFilename, ctx); err != nil {
										jobResult.Error = err
										dc.Logger.Printf("\tFailed to copy dumped file for pod %s in namespace %s: %v\n", pod.Name, namespace, err)
									} else {
										dc.Logger.Printf("\tSuccessfully copied dumped file for pod %s in namespace %s\n", pod.Name, namespace)
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
