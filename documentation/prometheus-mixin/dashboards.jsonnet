local dashboards = (import 'mixin.libsonnet').dashboards;

{
  [name]: dashboards[name]
  for name in std.objectFields(dashboards)
}
