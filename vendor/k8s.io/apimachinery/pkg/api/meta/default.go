/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package meta

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

// NewDefaultRESTMapperFromScheme instantiates a DefaultRESTMapper based on types registered in the given scheme.
func NewDefaultRESTMapperFromScheme(defaultGroupVersions []schema.GroupVersion, interfacesFunc VersionInterfacesFunc,
	importPathPrefix string, ignoredKinds, rootScoped sets.String, scheme *runtime.Scheme) *DefaultRESTMapper {

	mapper := NewDefaultRESTMapper(defaultGroupVersions, interfacesFunc)
	// enumerate all supported versions, get the kinds, and register with the mapper how to address
	// our resources.
	for _, gv := range defaultGroupVersions {
		for kind, oType := range scheme.KnownTypes(gv) {
			gvk := gv.WithKind(kind)
			// TODO: Remove import path check.
			// We check the import path because we currently stuff both "api" and "extensions" objects
			// into the same group within Scheme since Scheme has no notion of groups yet.
			if !strings.Contains(oType.PkgPath(), importPathPrefix) || ignoredKinds.Has(kind) {
				continue
			}
			scope := RESTScopeNamespace
			if rootScoped.Has(kind) {
				scope = RESTScopeRoot
			}
			mapper.Add(gvk, scope)
		}
	}
	return mapper
}
