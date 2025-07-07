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
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nginxinc/nginx-k8s-supportpkg/pkg/data_collector"
)

type Job struct {
	Name    string
	Timeout time.Duration
	Execute func(dc *data_collector.DataCollector, ctx context.Context, ch chan JobResult)
}

type JobResult struct {
	Files   map[string][]byte
	Error   error
	Skipped bool
}

func (j Job) Collect(dc *data_collector.DataCollector) (error, bool) {
	ch := make(chan JobResult, 1)

	ctx, cancel := context.WithTimeout(context.Background(), j.Timeout)
	defer cancel()

	dc.Logger.Printf("\tJob %s has started\n", j.Name)
	go j.Execute(dc, ctx, ch)

	select {
	case <-ctx.Done():
		dc.Logger.Printf("\tJob %s has timed out: %s\n---\n", j.Name, ctx.Err())
		err := fmt.Errorf("Context cancelled: %v", ctx.Err())
		return err, false

	case jobResults := <-ch:
		if jobResults.Error != nil {
			dc.Logger.Printf("\tJob %s has failed: %s\n", j.Name, jobResults.Error)
			return jobResults.Error, jobResults.Skipped
		}

		for fileName, fileValue := range jobResults.Files {
			err := os.MkdirAll(filepath.Dir(fileName), os.ModePerm)
			if err != nil {
				return fmt.Errorf("MkdirAll failed: %v", err), jobResults.Skipped
			}
			file, _ := os.Create(fileName)
			_, err = file.Write(fileValue)
			if err != nil {
				return fmt.Errorf("Write failed: %v", err), jobResults.Skipped
			}
			_ = file.Close()
			dc.Logger.Printf("\tJob %s wrote %d bytes to %s\n", j.Name, len(fileValue), fileName)
		}
		dc.Logger.Printf("\tJob %s completed successfully\n---\n", j.Name)
		return nil, jobResults.Skipped
	}
}
