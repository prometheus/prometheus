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

// Jest 27 ignores package.json "exports", so subpaths that only exist there
// (e.g. "entities/decode") fail to resolve. Fall back to Node's resolver,
// which honors "exports", when Jest's default resolution fails.
module.exports = (request, options) => {
  try {
    return options.defaultResolver(request, options);
  } catch (err) {
    try {
      return require.resolve(request, { paths: [options.basedir] });
    } catch {
      throw err;
    }
  }
};
