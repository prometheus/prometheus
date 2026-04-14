// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io"
	"net/http"
)

type debugWriterConfig struct {
	serverURL      string
	tarballName    string
	endPointGroups []endpointsGroup
}

func debugWrite(cfg debugWriterConfig) error {
	archiver, err := newTarGzFileWriter(cfg.tarballName)
	if err != nil {
		return fmt.Errorf("error creating a new archiver: %w", err)
	}

	for _, endPointGroup := range cfg.endPointGroups {
		for url, filename := range endPointGroup.urlToFilename {
			url := cfg.serverURL + url
			fmt.Println("collecting:", url)
			res, err := http.Get(url)
			if err != nil {
				return fmt.Errorf("error executing HTTP request: %w", err)
			}
			body, err := io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				return fmt.Errorf("error reading the response body: %w", err)
			}

			if endPointGroup.postProcess != nil {
				body, err = endPointGroup.postProcess(body)
				if err != nil {
					return fmt.Errorf("error post-processing HTTP response body: %w", err)
				}
			}
			if err := archiver.write(filename, body); err != nil {
				return fmt.Errorf("error writing into the archive: %w", err)
			}
		}
	}

	if err := archiver.close(); err != nil {
		return fmt.Errorf("error closing archive writer: %w", err)
	}

	fmt.Printf("Compiling debug information complete, all files written in %q.\n", cfg.tarballName)
	return nil
}
