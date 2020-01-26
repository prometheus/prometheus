## 0.4.0 (Unreleased)

## 0.3.0 (July 31, 2019)

IMPROVEMENTS

* Added `baremetal/apiversions.List` [GH-1577](https://github.com/gophercloud/gophercloud/pull/1577)
* Added `baremetal/apiversions.Get` [GH-1577](https://github.com/gophercloud/gophercloud/pull/1577)
* Added `compute/v2/extensions/servergroups.CreateOpts.Policy` [GH-1636](https://github.com/gophercloud/gophercloud/pull/1636)
* Added `identity/v3/extensions/trusts.Create` [GH-1644](https://github.com/gophercloud/gophercloud/pull/1644)
* Added `identity/v3/extensions/trusts.Delete` [GH-1644](https://github.com/gophercloud/gophercloud/pull/1644)
* Added `CreatedAt` and `UpdatedAt` to `networking/v2/extensions/layer3/floatingips.FloatingIP` [GH-1647](https://github.com/gophercloud/gophercloud/issues/1646)
* Added `CreatedAt` and `UpdatedAt` to `networking/v2/extensions/security/groups.SecGroup` [GH-1654](https://github.com/gophercloud/gophercloud/issues/1654)
* Added `CreatedAt` and `UpdatedAt` to `networking/v2/networks.Network` [GH-1657](https://github.com/gophercloud/gophercloud/issues/1657)
* Added `keymanager/v1/containers.CreateSecretRef` [GH-1659](https://github.com/gophercloud/gophercloud/issues/1659)
* Added `keymanager/v1/containers.DeleteSecretRef` [GH-1659](https://github.com/gophercloud/gophercloud/issues/1659)
* Added `sharedfilesystems/v2/shares.GetMetadata` [GH-1656](https://github.com/gophercloud/gophercloud/issues/1656)
* Added `sharedfilesystems/v2/shares.GetMetadatum` [GH-1656](https://github.com/gophercloud/gophercloud/issues/1656)
* Added `sharedfilesystems/v2/shares.SetMetadata` [GH-1656](https://github.com/gophercloud/gophercloud/issues/1656)
* Added `sharedfilesystems/v2/shares.UpdateMetadata` [GH-1656](https://github.com/gophercloud/gophercloud/issues/1656)
* Added `sharedfilesystems/v2/shares.DeleteMetadatum` [GH-1656](https://github.com/gophercloud/gophercloud/issues/1656)
* Added `sharedfilesystems/v2/sharetypes.IDFromName` [GH-1662](https://github.com/gophercloud/gophercloud/issues/1662)



BUG FIXES

* Changed `baremetal/v1/nodes.CleanStep.Args` from `map[string]string` to `map[string]interface{}` [GH-1638](https://github.com/gophercloud/gophercloud/pull/1638)
* Removed `URLPath` and `ExpectedCodes` from `loadbalancer/v2/monitors.ToMonitorCreateMap` since Octavia now provides default values when these fields are not specified [GH-1640](https://github.com/gophercloud/gophercloud/pull/1540)


## 0.2.0 (June 17, 2019)

IMPROVEMENTS

* Added `networking/v2/extensions/qos/rules.ListBandwidthLimitRules` [GH-1584](https://github.com/gophercloud/gophercloud/pull/1584)
* Added `networking/v2/extensions/qos/rules.GetBandwidthLimitRule` [GH-1584](https://github.com/gophercloud/gophercloud/pull/1584)
* Added `networking/v2/extensions/qos/rules.CreateBandwidthLimitRule` [GH-1584](https://github.com/gophercloud/gophercloud/pull/1584)
* Added `networking/v2/extensions/qos/rules.UpdateBandwidthLimitRule` [GH-1589](https://github.com/gophercloud/gophercloud/pull/1589)
* Added `networking/v2/extensions/qos/rules.DeleteBandwidthLimitRule` [GH-1590](https://github.com/gophercloud/gophercloud/pull/1590)
* Added `networking/v2/extensions/qos/policies.List` [GH-1591](https://github.com/gophercloud/gophercloud/pull/1591)
* Added `networking/v2/extensions/qos/policies.Get` [GH-1593](https://github.com/gophercloud/gophercloud/pull/1593)
* Added `networking/v2/extensions/qos/rules.ListDSCPMarkingRules` [GH-1594](https://github.com/gophercloud/gophercloud/pull/1594)
* Added `networking/v2/extensions/qos/policies.Create` [GH-1595](https://github.com/gophercloud/gophercloud/pull/1595)
* Added `compute/v2/extensions/diagnostics.Get` [GH-1592](https://github.com/gophercloud/gophercloud/pull/1592)
* Added `networking/v2/extensions/qos/policies.Update` [GH-1603](https://github.com/gophercloud/gophercloud/pull/1603)
* Added `networking/v2/extensions/qos/policies.Delete` [GH-1603](https://github.com/gophercloud/gophercloud/pull/1603)
* Added `networking/v2/extensions/qos/rules.CreateDSCPMarkingRule` [GH-1605](https://github.com/gophercloud/gophercloud/pull/1605)
* Added `networking/v2/extensions/qos/rules.UpdateDSCPMarkingRule` [GH-1605](https://github.com/gophercloud/gophercloud/pull/1605)
* Added `networking/v2/extensions/qos/rules.GetDSCPMarkingRule` [GH-1609](https://github.com/gophercloud/gophercloud/pull/1609)
* Added `networking/v2/extensions/qos/rules.DeleteDSCPMarkingRule` [GH-1609](https://github.com/gophercloud/gophercloud/pull/1609)
* Added `networking/v2/extensions/qos/rules.ListMinimumBandwidthRules` [GH-1615](https://github.com/gophercloud/gophercloud/pull/1615)
* Added `networking/v2/extensions/qos/rules.GetMinimumBandwidthRule` [GH-1615](https://github.com/gophercloud/gophercloud/pull/1615)
* Added `networking/v2/extensions/qos/rules.CreateMinimumBandwidthRule` [GH-1615](https://github.com/gophercloud/gophercloud/pull/1615)
* Added `Hostname` to `baremetalintrospection/v1/introspection.Data` [GH-1627](https://github.com/gophercloud/gophercloud/pull/1627)
* Added `networking/v2/extensions/qos/rules.UpdateMinimumBandwidthRule` [GH-1624](https://github.com/gophercloud/gophercloud/pull/1624)
* Added `networking/v2/extensions/qos/rules.DeleteMinimumBandwidthRule` [GH-1624](https://github.com/gophercloud/gophercloud/pull/1624)
* Added `networking/v2/extensions/qos/ruletypes.GetRuleType` [GH-1625](https://github.com/gophercloud/gophercloud/pull/1625)
* Added `Extra` to `baremetalintrospection/v1/introspection.Data` [GH-1611](https://github.com/gophercloud/gophercloud/pull/1611)
* Added `blockstorage/extensions/volumeactions.SetImageMetadata` [GH-1621](https://github.com/gophercloud/gophercloud/pull/1621)

BUG FIXES

* Updated `networking/v2/extensions/qos/rules.UpdateBandwidthLimitRule` to use return code 200 [GH-1606](https://github.com/gophercloud/gophercloud/pull/1606)
* Fixed bug in `compute/v2/extensions/schedulerhints.SchedulerHints.Query` where contents will now be marshalled to a string [GH-1620](https://github.com/gophercloud/gophercloud/pull/1620)

## 0.1.0 (May 27, 2019)

Initial tagged release. 
