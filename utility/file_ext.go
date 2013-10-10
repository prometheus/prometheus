// Copyright 2013 Prometheus Team
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

////////////////////////////////////////////////////////////////////////////////
// Useful file/filesystem related functions.
//

package utility

import (
    "fmt"
    "os"
)


func IsDir(dirPath string) (bool, error) {
    finfo, err := os.Stat(dirPath)
    if err != nil {
        return false, err
    }
    if !finfo.IsDir() {
        return false, fmt.Errorf("%s not a directory", dirPath)
    }
    return true, nil
}




