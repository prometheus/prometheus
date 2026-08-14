local dashboards = (import 'mixin.libsonnet').grafanaDashboards;

{
  [name]: dashboards[name]
  for name in std.objectFields(dashboards)
}
