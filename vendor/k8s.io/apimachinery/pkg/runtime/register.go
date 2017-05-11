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

package runtime

import "k8s.io/apimachinery/pkg/runtime/schema"

// SetGroupVersionKind satisfies the ObjectKind interface for all objects that embed TypeMeta
func (obj *TypeMeta) SetGroupVersionKind(gvk schema.GroupVersionKind) {
	obj.APIVersion, obj.Kind = gvk.ToAPIVersionAndKind()
}

// GroupVersionKind satisfies the ObjectKind interface for all objects that embed TypeMeta
func (obj *TypeMeta) GroupVersionKind() schema.GroupVersionKind {
	return schema.FromAPIVersionAndKind(obj.APIVersion, obj.Kind)
}

func (obj *Unknown) GetObjectKind() schema.ObjectKind { return &obj.TypeMeta }

// GetObjectKind implements Object for VersionedObjects, returning an empty ObjectKind
// interface if no objects are provided, or the ObjectKind interface of the object in the
// highest array position.
func (obj *VersionedObjects) GetObjectKind() schema.ObjectKind {
	last := obj.Last()
	if last == nil {
		return schema.EmptyObjectKind
	}
	return last.GetObjectKind()
}

// First returns the leftmost object in the VersionedObjects array, which is usually the
// object as serialized on the wire.
func (obj *VersionedObjects) First() Object {
	if len(obj.Objects) == 0 {
		return nil
	}
	return obj.Objects[0]
}

// Last is the rightmost object in the VersionedObjects array, which is the object after
// all transformations have been applied. This is the same object that would be returned
// by Decode in a normal invocation (without VersionedObjects in the into argument).
func (obj *VersionedObjects) Last() Object {
	if len(obj.Objects) == 0 {
		return nil
	}
	return obj.Objects[len(obj.Objects)-1]
}
