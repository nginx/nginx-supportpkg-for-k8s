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

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/jobs"
	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/version"
	"github.com/spf13/cobra"
)

func Execute() {

	var product string
	var jobList []jobs.Job
	collector := data_collector.DataCollector{}

	var rootCmd = &cobra.Command{
		Use:   "nginx-supportpkg",
		Short: "nginx-supportpkg - a tool to create Ingress Controller diagnostics package",
		Long:  `nginx-supportpkg - a tool to create Ingress Controller diagnostics package`,
		Run: func(cmd *cobra.Command, args []string) {
			startTime := time.Now()
			err := data_collector.NewDataCollector(&collector)
			if err != nil {
				fmt.Println(fmt.Errorf("unable to start data collector: %s", err))
				os.Exit(1)
			}

			collector.Logger.Printf("Starting kubectl-nginx-supportpkg - version: %s - build: %s", version.Version, version.Build)
			collector.Logger.Printf("Input args are %v", os.Args)

			switch product {
			case "nic":
				jobList = slices.Concat(jobs.CommonJobList(), jobs.NICJobList())
			case "ngf":
				jobList = slices.Concat(jobs.CommonJobList(), jobs.NGFJobList())
			case "ngx":
				jobList = slices.Concat(jobs.CommonJobList(), jobs.NGXJobList())
			case "nim":
				jobList = slices.Concat(jobs.CommonJobList(), jobs.NIMJobList())
			default:
				fmt.Printf("Error: product must be in the following list: [nic, ngf, ngx, nim]\n")
				os.Exit(1)
			}

			if collector.AllNamespacesExist() {
				failedJobs := 0
				totalJobs := len(jobList)
				var jobTimings []data_collector.JobInfo
				const jobNameWidth = 30

				for _, job := range jobList {
					fmt.Printf("Running job %s", job.Name)
					// Calculate number of dots needed
					dots := jobNameWidth - len(job.Name)
					if dots < 0 {
						dots = 0
					}
					fmt.Printf("%s ", strings.Repeat(".", dots))
					// Record job start and end time to calculate duration
					jobStartTime := time.Now()
					err, skipped, files := job.Collect(&collector)
					jobEndTime := time.Now()
					duration := jobEndTime.Sub(jobStartTime)

					// Create job info record
					jobInfo := data_collector.JobInfo{
						Name:      job.Name,
						StartTime: jobStartTime.UTC().Format(time.RFC3339Nano),
						EndTime:   jobEndTime.UTC().Format(time.RFC3339Nano),
						Duration:  duration.String(),
						Files:     files,
					}

					if skipped {
						fmt.Print(" SKIPPED\n")
					} else if err != nil {
						fmt.Printf(" FAILED: %s\n", err)
						failedJobs++
					} else {
						fmt.Print(" COMPLETED\n")
					}

					jobTimings = append(jobTimings, jobInfo)
				}

				// Generate manifest with job timings
				manifestData, err := collector.GenerateManifest(product, startTime, totalJobs, failedJobs, jobTimings)
				if err != nil {
					fmt.Printf("Warning: Failed to generate manifest: %v\n", err)
				} else {
					// Save manifest to base directory
					manifestPath := filepath.Join(collector.BaseDir, "manifest.json")
					err = os.WriteFile(manifestPath, manifestData, 0644)
					if err != nil {
						fmt.Printf("Warning: Failed to write manifest: %v\n", err)
					}
				}

				tarFile, err := collector.WrapUp(product)
				if err != nil {
					fmt.Println(fmt.Errorf("error when wrapping up: %s", err))
					os.Exit(1)
				} else {
					if failedJobs == 0 {
						fmt.Printf("Supportpkg successfully generated: %s\n", tarFile)
					} else {
						fmt.Printf("WARNING: %d failed job(s)\n", failedJobs)
						fmt.Printf("Supportpkg generated with warnings: %s\n", tarFile)
					}

				}
			} else {
				fmt.Println(" Error: Some namespaces do not exist")
			}
		},
	}

	rootCmd.Flags().StringSliceVarP(&collector.Namespaces, "namespace", "n", []string{}, "list of namespaces to collect information from")
	if err := rootCmd.MarkFlagRequired("namespace"); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	rootCmd.Flags().StringVarP(&product, "product", "p", "", "products to collect information from")
	if err := rootCmd.MarkFlagRequired("product"); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	rootCmd.Flags().BoolVarP(&collector.ExcludeDBData, "exclude-db-data", "d", false, "exclude DB data collection")
	rootCmd.Flags().BoolVarP(&collector.ExcludeTimeSeriesData, "exclude-time-series-data", "t", false, "exclude time series data collection")

	versionStr := "nginx-supportpkg - version: " + version.Version + " - build: " + version.Build + "\n"
	rootCmd.SetVersionTemplate(versionStr)
	rootCmd.Version = versionStr

	rootCmd.SetUsageTemplate(
		versionStr +
			"Usage:" +
			"\n nginx-supportpkg -h|--help" +
			"\n nginx-supportpkg -v|--version" +
			"\n nginx-supportpkg [-n|--namespace] ns1 [-n|--namespace] ns2 [-p|--product] [nic,ngf,ngx,nim]" +
			"\n nginx-supportpkg [-n|--namespace] ns1,ns2 [-p|--product] [nic,ngf,ngx,nim]" +
			"\n nginx-supportpkg [-n|--namespace] ns1 [-n|--namespace] ns2 [-p|--product] [nim] [-d|--exclude-db-data] [-t|--exclude-time-series-data] \n")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
