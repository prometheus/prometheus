// Copyright 2024 Prometheus Team
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

package io_prometheus_client //nolint

import (
	"errors"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type nopTypeRegistry struct{}

func (nopTypeRegistry) RegisterMessage(protoreflect.MessageType) error     { return nil }
func (nopTypeRegistry) RegisterEnum(protoreflect.EnumType) error           { return nil }
func (nopTypeRegistry) RegisterExtension(protoreflect.ExtensionType) error { return nil }

type nopFileRegistry struct{}

func (nopFileRegistry) FindFileByPath(string) (protoreflect.FileDescriptor, error) {
	return nil, errors.New("nop file registry, shouldn't be used for querying")
}

func (nopFileRegistry) FindDescriptorByName(protoreflect.FullName) (protoreflect.Descriptor, error) {
	return nil, errors.New("nop file registry, shouldn't be used for querying")
}
func (nopFileRegistry) RegisterFile(protoreflect.FileDescriptor) error { return nil }
