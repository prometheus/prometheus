/*
Copyright (c) 2017 VMware, Inc. All Rights Reserved.

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

package rule

import (
	"context"
	"flag"
	"fmt"

	"github.com/vmware/govmomi/govc/flags"
	"github.com/vmware/govmomi/vim25/types"
)

type InfoFlag struct {
	*flags.ClusterFlag

	rules []types.BaseClusterRuleInfo

	name string
}

func NewInfoFlag(ctx context.Context) (*InfoFlag, context.Context) {
	f := &InfoFlag{}
	f.ClusterFlag, ctx = flags.NewClusterFlag(ctx)
	return f, ctx
}

func (f *InfoFlag) Register(ctx context.Context, fs *flag.FlagSet) {
	f.ClusterFlag.Register(ctx, fs)

	fs.StringVar(&f.name, "name", "", "Cluster rule name")
}

func (f *InfoFlag) Process(ctx context.Context) error {
	return f.ClusterFlag.Process(ctx)
}

func (f *InfoFlag) Rules(ctx context.Context) ([]types.BaseClusterRuleInfo, error) {
	if f.rules != nil {
		return f.rules, nil
	}

	cluster, err := f.Cluster()
	if err != nil {
		return nil, err
	}

	config, err := cluster.Configuration(ctx)
	if err != nil {
		return nil, err
	}

	f.rules = config.Rule

	return f.rules, nil
}

type ClusterRuleInfo struct {
	info types.BaseClusterRuleInfo

	refs *[]types.ManagedObjectReference

	kind string
}

func (f *InfoFlag) Rule(ctx context.Context) (*ClusterRuleInfo, error) {
	rules, err := f.Rules(ctx)
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		if rule.GetClusterRuleInfo().Name != f.name {
			continue
		}

		r := &ClusterRuleInfo{info: rule}

		switch info := rule.(type) {
		case *types.ClusterAffinityRuleSpec:
			r.refs = &info.Vm
			r.kind = "VirtualMachine"
		case *types.ClusterAntiAffinityRuleSpec:
			r.refs = &info.Vm
			r.kind = "VirtualMachine"
		}

		return r, nil
	}

	return nil, fmt.Errorf("rule %q not found", f.name)
}

func (f *InfoFlag) Apply(ctx context.Context, update types.ArrayUpdateSpec, info types.BaseClusterRuleInfo) error {
	spec := &types.ClusterConfigSpecEx{
		RulesSpec: []types.ClusterRuleSpec{
			{
				ArrayUpdateSpec: update,
				Info:            info,
			},
		},
	}

	return f.Reconfigure(ctx, spec)
}

type SpecFlag struct {
	types.ClusterRuleInfo
	types.ClusterVmHostRuleInfo
	types.ClusterAffinityRuleSpec
	types.ClusterAntiAffinityRuleSpec
}

func (s *SpecFlag) Register(ctx context.Context, f *flag.FlagSet) {
	f.Var(flags.NewOptionalBool(&s.Enabled), "enable", "Enable rule")
	f.Var(flags.NewOptionalBool(&s.Mandatory), "mandatory", "Enforce rule compliance")

	f.StringVar(&s.VmGroupName, "vm-group", "", "VM group name")
	f.StringVar(&s.AffineHostGroupName, "host-affine-group", "", "Host affine group name")
	f.StringVar(&s.AntiAffineHostGroupName, "host-anti-affine-group", "", "Host anti-affine group name")
}
