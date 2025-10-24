local grafana = import 'github.com/grafana/grafonnet/gen/grafonnet-latest/main.libsonnet';
local dashboard = grafana.dashboard;
local prometheus = grafana.query.prometheus;
local variable = dashboard.variable;
local panel = grafana.panel;
local row = panel.row;

{
  grafanaDashboards+:: {

    local panelTimeSeriesStdOptions =
      {}
      + panel.timeSeries.queryOptions.withDatasource('prometheus', '$datasource')
      + panel.timeSeries.fieldConfig.defaults.custom.withFillOpacity(10)
      + panel.timeSeries.fieldConfig.defaults.custom.withShowPoints('never')
      + panel.timeSeries.options.tooltip.withMode('multi')
    ,

    local panelTimeSeriesStacking =
      {}
      + panel.timeSeries.fieldConfig.defaults.custom.withFillOpacity(100)
      + panel.timeSeries.fieldConfig.defaults.custom.withLineWidth(0)
      + panel.timeSeries.fieldConfig.defaults.custom.stacking.withMode('normal')
    ,

    'prometheus.json':

      local showMultiCluster = $._config.showMultiCluster;

      local datasourceVariable =
        variable.datasource.new('datasource', 'prometheus')
        + variable.datasource.generalOptions.withLabel('Data source')
        + variable.datasource.generalOptions.withCurrent('default')
        + variable.datasource.generalOptions.showOnDashboard.withLabelAndValue()
      ;

      local clusterVariable =
        variable.query.new('cluster')
        + variable.query.generalOptions.withLabel('cluster')
        + variable.query.withDatasourceFromVariable(datasourceVariable)
        + variable.query.refresh.onTime()
        + variable.query.withSort(type='alphabetical', asc=false)
        + variable.query.selectionOptions.withIncludeAll(true, '.+')
        + variable.query.selectionOptions.withMulti(true)
        + variable.query.generalOptions.withCurrent('$__all')
        + variable.query.queryTypes.withLabelValues($._config.clusterLabel, metric='prometheus_build_info{%(prometheusSelector)s}' % $._config)
        + variable.datasource.generalOptions.showOnDashboard.withLabelAndValue()
      ;

      local jobVariable =
        variable.query.new('job')
        + variable.query.generalOptions.withLabel('job')
        + variable.query.withDatasourceFromVariable(datasourceVariable)
        + variable.query.refresh.onTime()
        + variable.query.withSort(type='alphabetical', asc=false)
        + variable.query.selectionOptions.withIncludeAll(true, '.+')
        + variable.query.selectionOptions.withMulti(true)
        + if showMultiCluster then
          variable.query.queryTypes.withLabelValues('job', metric='prometheus_build_info{%(clusterLabel)s=~"$cluster"}' % $._config)
        else
          variable.query.queryTypes.withLabelValues('job', metric='prometheus_build_info{%(prometheusSelector)s}' % $._config)
      ;

      local instanceVariable =
        variable.query.new('instance')
        + variable.query.generalOptions.withLabel('instance')
        + variable.query.withDatasourceFromVariable(datasourceVariable)
        + variable.query.refresh.onTime()
        + variable.query.withSort(type='alphabetical', asc=false)
        + variable.query.selectionOptions.withIncludeAll(true, '.+')
        + variable.query.selectionOptions.withMulti(true)
        + if showMultiCluster then
          variable.query.queryTypes.withLabelValues('instance', metric='prometheus_build_info{%(clusterLabel)s=~"$cluster", job=~"$job"}' % $._config)
        else
          variable.query.queryTypes.withLabelValues('instance', metric='prometheus_build_info{job=~"$job"}')
      ;

      local prometheusStats =
        panel.table.new('Prometheus Stats')
        + panel.table.queryOptions.withDatasource('prometheus', '$datasource')
        + panel.table.standardOptions.withUnit('short')
        + panel.table.standardOptions.withDecimals(2)
        + panel.table.standardOptions.withDisplayName('')
        + panel.table.standardOptions.withOverrides([
          panel.table.standardOptions.override.byName.new('Time')
          + panel.table.standardOptions.override.byName.withProperty('displayName', 'Time')
          + panel.table.standardOptions.override.byName.withProperty('custom.align', null)
          + panel.table.standardOptions.override.byName.withProperty('custom.hidden', 'true'),
          panel.table.standardOptions.override.byName.new('cluster')
          + panel.table.standardOptions.override.byName.withProperty('custom.align', null)
          + panel.table.standardOptions.override.byName.withProperty('unit', 'short')
          + panel.table.standardOptions.override.byName.withProperty('decimals', 2)
          + if showMultiCluster then panel.table.standardOptions.override.byName.withProperty('displayName', 'Cluster') else {},
          panel.table.standardOptions.override.byName.new('job')
          + panel.table.standardOptions.override.byName.withProperty('custom.align', null)
          + panel.table.standardOptions.override.byName.withProperty('unit', 'short')
          + panel.table.standardOptions.override.byName.withProperty('decimals', 2)
          + panel.table.standardOptions.override.byName.withProperty('displayName', 'Job'),
          panel.table.standardOptions.override.byName.new('instance')
          + panel.table.standardOptions.override.byName.withProperty('displayName', 'Instance')
          + panel.table.standardOptions.override.byName.withProperty('custom.align', null)
          + panel.table.standardOptions.override.byName.withProperty('unit', 'short')
          + panel.table.standardOptions.override.byName.withProperty('decimals', 2),
          panel.table.standardOptions.override.byName.new('version')
          + panel.table.standardOptions.override.byName.withProperty('displayName', 'Version')
          + panel.table.standardOptions.override.byName.withProperty('custom.align', null)
          + panel.table.standardOptions.override.byName.withProperty('unit', 'short')
          + panel.table.standardOptions.override.byName.withProperty('decimals', 2),
          panel.table.standardOptions.override.byName.new('Value #A')
          + panel.table.standardOptions.override.byName.withProperty('displayName', 'Count')
          + panel.table.standardOptions.override.byName.withProperty('custom.align', null)
          + panel.table.standardOptions.override.byName.withProperty('unit', 'short')
          + panel.table.standardOptions.override.byName.withProperty('decimals', 2)
          + panel.table.standardOptions.override.byName.withProperty('custom.hidden', 'true'),
          panel.table.standardOptions.override.byName.new('Value #B')
          + panel.table.standardOptions.override.byName.withProperty('displayName', 'Uptime')
          + panel.table.standardOptions.override.byName.withProperty('custom.align', null)
          + panel.table.standardOptions.override.byName.withProperty('unit', 's'),
        ])
        + if showMultiCluster then
          panel.table.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'count by (cluster, job, instance, version) (prometheus_build_info{%(clusterLabel)s=~"$cluster", job=~"$job", instance=~"$instance"})' % $._config
            )
            + prometheus.withFormat('table')
            + prometheus.withInstant(true)
            + prometheus.withLegendFormat(''),
            prometheus.new(
              '$datasource',
              'max by (cluster, job, instance) (time() - process_start_time_seconds{%(clusterLabel)s=~"$cluster", job=~"$job", instance=~"$instance"})' % $._config
            )
            + prometheus.withFormat('table')
            + prometheus.withInstant(true)
            + prometheus.withLegendFormat(''),
          ])
        else
          panel.table.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'count by (job, instance, version) (prometheus_build_info{job=~"$job", instance=~"$instance"})'
            )
            + prometheus.withFormat('table')
            + prometheus.withInstant(true)
            + prometheus.withLegendFormat(''),
            prometheus.new(
              '$datasource',
              'max by (job, instance) (time() - process_start_time_seconds{job=~"$job", instance=~"$instance"})'
            )
            + prometheus.withFormat('table')
            + prometheus.withInstant(true)
            + prometheus.withLegendFormat(''),
          ])
      ;

      local targetSync =
        panel.timeSeries.new('Target Sync')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withSort('desc')
        + panel.timeSeries.standardOptions.withMin(0)
        + panel.timeSeries.standardOptions.withUnit('ms')
        + if showMultiCluster then
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'sum(rate(prometheus_target_sync_length_seconds_sum{%(clusterLabel)s=~"$cluster",job=~"$job",instance=~"$instance"}[5m])) by (%(clusterLabel)s, job, scrape_job, instance) * 1e3' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{job}}:{{instance}}:{{scrape_job}}' % $._config),
          ])
        else
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'sum(rate(prometheus_target_sync_length_seconds_sum{job=~"$job",instance=~"$instance"}[5m])) by (scrape_job) * 1e3'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{scrape_job}}'),
          ])
      ;

      local targets =
        panel.timeSeries.new('Targets')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withSort('desc')
        + panel.timeSeries.standardOptions.withMin(0)
        + panelTimeSeriesStacking
        + panel.timeSeries.standardOptions.withUnit('short')
        + if showMultiCluster then
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'sum by (%(clusterLabel)s, job, instance) (prometheus_sd_discovered_targets{%(clusterLabel)s=~"$cluster", job=~"$job",instance=~"$instance"})' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{job}}:{{instance}}' % $._config),
          ])
        else
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'sum(prometheus_sd_discovered_targets{job=~"$job",instance=~"$instance"})'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('Targets'),
          ])
      ;

      local averageScrapeIntervalDuration =
        panel.timeSeries.new('Average Scrape Interval Duration')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withSort('desc')
        + panel.timeSeries.standardOptions.withMin(0)
        + panel.timeSeries.standardOptions.withUnit('ms')
        + if showMultiCluster then
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'rate(prometheus_target_interval_length_seconds_sum{%(clusterLabel)s=~"$cluster", job=~"$job",instance=~"$instance"}[5m]) / rate(prometheus_target_interval_length_seconds_count{%(clusterLabel)s=~"$cluster", job=~"$job",instance=~"$instance"}[5m]) * 1e3' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{job}}:{{instance}} {{interval}} configured' % $._config),
          ])
        else
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'rate(prometheus_target_interval_length_seconds_sum{job=~"$job",instance=~"$instance"}[5m]) / rate(prometheus_target_interval_length_seconds_count{job=~"$job",instance=~"$instance"}[5m]) * 1e3'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{interval}} configured'),
          ])
      ;

      local scrapeFailures =
        panel.timeSeries.new('Scrape failures')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withSort('desc')
        + panel.timeSeries.standardOptions.withMin(0)
        + panelTimeSeriesStacking
        + panel.timeSeries.standardOptions.withUnit('short')
        + if showMultiCluster then
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'sum by (%(clusterLabel)s, job, instance) (rate(prometheus_target_scrapes_exceeded_body_size_limit_total{%(clusterLabel)s=~"$cluster",job=~"$job",instance=~"$instance"}[1m]))' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('exceeded body size limit: {{%(clusterLabel)s}} {{job}} {{instance}}' % $._config),
            prometheus.new(
              '$datasource',
              'sum by (%(clusterLabel)s, job, instance) (rate(prometheus_target_scrapes_exceeded_sample_limit_total{%(clusterLabel)s=~"$cluster",job=~"$job",instance=~"$instance"}[1m]))' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('exceeded sample limit: {{%(clusterLabel)s}} {{job}} {{instance}}' % $._config),
            prometheus.new(
              '$datasource',
              'sum by (%(clusterLabel)s, job, instance) (rate(prometheus_target_scrapes_sample_duplicate_timestamp_total{%(clusterLabel)s=~"$cluster",job=~"$job",instance=~"$instance"}[1m]))' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('duplicate timestamp: {{%(clusterLabel)s}} {{job}} {{instance}}' % $._config),
            prometheus.new(
              '$datasource',
              'sum by (%(clusterLabel)s, job, instance) (rate(prometheus_target_scrapes_sample_out_of_bounds_total{%(clusterLabel)s=~"$cluster",job=~"$job",instance=~"$instance"}[1m]))' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('out of bounds: {{%(clusterLabel)s}} {{job}} {{instance}}' % $._config),
            prometheus.new(
              '$datasource',
              'sum by (%(clusterLabel)s, job, instance) (rate(prometheus_target_scrapes_sample_out_of_order_total{%(clusterLabel)s=~"$cluster",job=~"$job",instance=~"$instance"}[1m]))' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('out of order: {{%(clusterLabel)s}} {{job}} {{instance}}' % $._config),
          ])
        else
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'sum by (job) (rate(prometheus_target_scrapes_exceeded_body_size_limit_total[1m]))'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('exceeded body size limit: {{job}}'),
            prometheus.new(
              '$datasource',
              'sum by (job) (rate(prometheus_target_scrapes_exceeded_sample_limit_total[1m]))'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('exceeded sample limit: {{job}}'),
            prometheus.new(
              '$datasource',
              'sum by (job) (rate(prometheus_target_scrapes_sample_duplicate_timestamp_total[1m]))'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('duplicate timestamp: {{job}}'),
            prometheus.new(
              '$datasource',
              'sum by (job) (rate(prometheus_target_scrapes_sample_out_of_bounds_total[1m]))'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('out of bounds: {{job}}'),
            prometheus.new(
              '$datasource',
              'sum by (job) (rate(prometheus_target_scrapes_sample_out_of_order_total[1m]))'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('out of order: {{job}}'),
          ])
      ;

      local appendedSamples =
        panel.timeSeries.new('Appended Samples')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withSort('desc')
        + panel.timeSeries.standardOptions.withMin(0)
        + panelTimeSeriesStacking
        + panel.timeSeries.standardOptions.withUnit('short')
        + if showMultiCluster then
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'rate(prometheus_tsdb_head_samples_appended_total{%(clusterLabel)s=~"$cluster", job=~"$job",instance=~"$instance"}[5m])' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{%(clusterLabel)s}} {{job}} {{instance}}' % $._config),
          ])
        else
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'rate(prometheus_tsdb_head_samples_appended_total{job=~"$job",instance=~"$instance"}[5m])'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{job}} {{instance}}'),
          ])
      ;

      local headSeries =
        panel.timeSeries.new('Head Series')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withSort('desc')
        + panel.timeSeries.standardOptions.withMin(0)
        + panelTimeSeriesStacking
        + panel.timeSeries.standardOptions.withUnit('short')
        + if showMultiCluster then
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'prometheus_tsdb_head_series{%(clusterLabel)s=~"$cluster",job=~"$job",instance=~"$instance"}' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{%(clusterLabel)s}} {{job}} {{instance}} head series' % $._config),
          ])
        else
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'prometheus_tsdb_head_series{job=~"$job",instance=~"$instance"}'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{job}} {{instance}} head series'),
          ])
      ;

      local headChunks =
        panel.timeSeries.new('Head Chunks')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withSort('desc')
        + panel.timeSeries.standardOptions.withMin(0)
        + panelTimeSeriesStacking
        + panel.timeSeries.standardOptions.withUnit('short')
        + if showMultiCluster then
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'prometheus_tsdb_head_chunks{%(clusterLabel)s=~"$cluster",job=~"$job",instance=~"$instance"}' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{%(clusterLabel)s}} {{job}} {{instance}} head chunks' % $._config),
          ])
        else
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'prometheus_tsdb_head_chunks{job=~"$job",instance=~"$instance"}'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{job}} {{instance}} head chunks'),
          ])
      ;

      local queryRate =
        panel.timeSeries.new('Query Rate')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withSort('desc')
        + panel.timeSeries.standardOptions.withMin(0)
        + panelTimeSeriesStacking
        + panel.timeSeries.standardOptions.withUnit('short')
        + if showMultiCluster then
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'rate(prometheus_engine_query_duration_seconds_count{%(clusterLabel)s=~"$cluster",job=~"$job",instance=~"$instance",slice="inner_eval"}[5m])' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{%(clusterLabel)s}} {{job}} {{instance}}' % $._config),
          ])
        else
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'rate(prometheus_engine_query_duration_seconds_count{job=~"$job",instance=~"$instance",slice="inner_eval"}[5m])'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{job}} {{instance}}'),
          ])
      ;

      local stageDuration =
        panel.timeSeries.new('Stage Duration')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withSort('desc')
        + panel.timeSeries.standardOptions.withMin(0)
        + panelTimeSeriesStacking
        + panel.timeSeries.standardOptions.withUnit('ms')
        + if showMultiCluster then
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'max by (slice) (prometheus_engine_query_duration_seconds{quantile="0.9",%(clusterLabel)s=~"$cluster", job=~"$job",instance=~"$instance"}) * 1e3' % $._config
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{slice}}'),
          ])
        else
          panel.timeSeries.queryOptions.withTargets([
            prometheus.new(
              '$datasource',
              'max by (slice) (prometheus_engine_query_duration_seconds{quantile="0.9",job=~"$job",instance=~"$instance"}) * 1e3'
            )
            + prometheus.withFormat('time_series')
            + prometheus.withLegendFormat('{{slice}}'),
          ])
      ;

      dashboard.new('%(prefix)sOverview' % $._config.grafanaPrometheus)
      + dashboard.time.withFrom('now-1h')
      + dashboard.withTags($._config.grafanaPrometheus.tags)
      + dashboard.withUid('9fa0d141-d019-4ad7-8bc5-42196ee308bd')
      + dashboard.timepicker.withRefreshIntervals($._config.grafanaPrometheus.refresh)
      + dashboard.withVariables(std.prune([
        datasourceVariable,
        if showMultiCluster then clusterVariable,
        jobVariable,
        instanceVariable,
      ]))
      + dashboard.withPanels(
        grafana.util.grid.makeGrid([
          row.new('Prometheus Stats')
          + row.withPanels([
            prometheusStats,
          ]),
        ], panelWidth=24, panelHeight=7)
        +
        grafana.util.grid.makeGrid([
          row.new('Discovery')
          + row.withPanels([
            targetSync,
            targets,
          ]),
        ], panelWidth=12, panelHeight=7, startY=8)
        +
        grafana.util.grid.makeGrid([
          row.new('Retrieval')
          + row.withPanels([
            averageScrapeIntervalDuration,
            scrapeFailures,
            appendedSamples,
          ]),
        ], panelWidth=8, panelHeight=7, startY=16)
        +
        grafana.util.grid.makeGrid([
          row.new('Storage')
          + row.withPanels([
            headSeries,
            headChunks,
          ]),
          row.new('Query')
          + row.withPanels([
            queryRate,
            stageDuration,
          ]),
        ], panelWidth=12, panelHeight=7, startY=24)
      ),
    // Remote write specific dashboard.
    'prometheus-remote-write.json':

      local datasourceVariable =
        variable.datasource.new('datasource', 'prometheus')
        + variable.datasource.generalOptions.withCurrent('default')
        + variable.datasource.generalOptions.showOnDashboard.withLabelAndValue()
      ;

      local clusterVariable =
        variable.query.new('cluster')
        + variable.query.withDatasourceFromVariable(datasourceVariable)
        + variable.query.refresh.onTime()
        + variable.query.selectionOptions.withIncludeAll(true)
        + variable.query.generalOptions.withCurrent('$__all')
        + variable.query.queryTypes.withLabelValues($._config.clusterLabel, metric='prometheus_build_info')
        + variable.datasource.generalOptions.showOnDashboard.withLabelAndValue()
      ;

      local instanceVariable =
        variable.query.new('instance')
        + variable.query.withDatasourceFromVariable(datasourceVariable)
        + variable.query.refresh.onTime()
        + variable.query.selectionOptions.withIncludeAll(true)
        + variable.query.queryTypes.withLabelValues('instance', metric='prometheus_build_info{%(clusterLabel)s=~"$cluster"}' % $._config)
      ;

      local urlVariable =
        variable.query.new('url')
        + variable.query.withDatasourceFromVariable(datasourceVariable)
        + variable.query.refresh.onTime()
        + variable.query.selectionOptions.withIncludeAll(true)
        + variable.query.queryTypes.withLabelValues('url', metric='prometheus_remote_storage_shards{%(clusterLabel)s=~"$cluster", instance=~"$instance"}' % $._config)
      ;

      local timestampComparison =
        panel.timeSeries.new('Highest Enqueued Timestamp vs. Highest Timestamp Sent')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.standardOptions.withUnit('short')
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            |||
              (
                prometheus_remote_storage_queue_highest_timestamp_seconds{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}
              -
                prometheus_remote_storage_queue_highest_sent_timestamp_seconds{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}
              )
            ||| % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}::{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local timestampComparisonRate =
        panel.timeSeries.new('Rate[5m]')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.standardOptions.withUnit('short')
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            |||
              clamp_min(
                rate(prometheus_remote_storage_queue_highest_timestamp_seconds{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m])
              -
                rate(prometheus_remote_storage_queue_highest_sent_timestamp_seconds{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m])
              , 0)
            ||| % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local samplesRate =
        panel.timeSeries.new('Rate, in vs. succeeded or dropped [5m]')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.standardOptions.withUnit('short')
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            |||
              rate(
                prometheus_remote_storage_samples_in_total{%(clusterLabel)s=~"$cluster", instance=~"$instance"}[5m])
              -
                ignoring(remote_name, url) group_right(instance) (rate(prometheus_remote_storage_succeeded_samples_total{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m]) or rate(prometheus_remote_storage_samples_total{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m]))
              -
                (rate(prometheus_remote_storage_dropped_samples_total{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m]) or rate(prometheus_remote_storage_samples_dropped_total{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m]))
            ||| % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local currentShards =
        panel.timeSeries.new('Current Shards')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.standardOptions.withUnit('short')
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'prometheus_remote_storage_shards{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local maxShards =
        panel.timeSeries.new('Max Shards')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.standardOptions.withUnit('short')
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'prometheus_remote_storage_shards_max{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local minShards =
        panel.timeSeries.new('Min Shards')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.standardOptions.withUnit('short')
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'prometheus_remote_storage_shards_min{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local desiredShards =
        panel.timeSeries.new('Desired Shards')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.standardOptions.withUnit('short')
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'prometheus_remote_storage_shards_desired{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local shardsCapacity =
        panel.timeSeries.new('Shard Capacity')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.standardOptions.withUnit('short')
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'prometheus_remote_storage_shard_capacity{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local pendingSamples =
        panel.timeSeries.new('Pending Samples')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.standardOptions.withUnit('short')
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'prometheus_remote_storage_pending_samples{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"} or prometheus_remote_storage_samples_pending{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local walSegment =
        panel.timeSeries.new('TSDB Current Segment')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withMode('single')
        + panel.timeSeries.fieldConfig.defaults.custom.withFillOpacity(0)
        + panel.timeSeries.standardOptions.withUnit('none')
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'prometheus_tsdb_wal_segment_current{%(clusterLabel)s=~"$cluster", instance=~"$instance"}' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}}' % $._config),
        ]);

      local queueSegment =
        panel.timeSeries.new('Remote Write Current Segment')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withMode('single')
        + panel.timeSeries.fieldConfig.defaults.custom.withFillOpacity(0)
        + panel.timeSeries.standardOptions.withUnit('none')
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'prometheus_wal_watcher_current_segment{%(clusterLabel)s=~"$cluster", instance=~"$instance"}' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{consumer}}' % $._config),
        ]);

      local droppedSamples =
        panel.timeSeries.new('Dropped Samples')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withMode('single')
        + panel.timeSeries.fieldConfig.defaults.custom.withFillOpacity(0)
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'rate(prometheus_remote_storage_dropped_samples_total{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m]) or rate(prometheus_remote_storage_samples_dropped_total{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m])' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local failedSamples =
        panel.timeSeries.new('Failed Samples')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withMode('single')
        + panel.timeSeries.fieldConfig.defaults.custom.withFillOpacity(0)
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'rate(prometheus_remote_storage_failed_samples_total{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m]) or rate(prometheus_remote_storage_samples_failed_total{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m])' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local retriedSamples =
        panel.timeSeries.new('Retried Samples')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withMode('single')
        + panel.timeSeries.fieldConfig.defaults.custom.withFillOpacity(0)
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'rate(prometheus_remote_storage_retried_samples_total{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m]) or rate(prometheus_remote_storage_samples_retried_total{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m])' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      local enqueueRetries =
        panel.timeSeries.new('Enqueue Retries')
        + panelTimeSeriesStdOptions
        + panel.timeSeries.options.tooltip.withMode('single')
        + panel.timeSeries.fieldConfig.defaults.custom.withFillOpacity(0)
        + panel.timeSeries.queryOptions.withTargets([
          prometheus.new(
            '$datasource',
            'rate(prometheus_remote_storage_enqueue_retries_total{%(clusterLabel)s=~"$cluster", instance=~"$instance", url=~"$url"}[5m])' % $._config
          )
          + prometheus.withFormat('time_series')
          + prometheus.withIntervalFactor(2)
          + prometheus.withLegendFormat('{{%(clusterLabel)s}}:{{instance}} {{remote_name}}:{{url}}' % $._config),
        ]);

      dashboard.new('%(prefix)sRemote Write' % $._config.grafanaPrometheus)
      + dashboard.time.withFrom('now-1h')
      + dashboard.withTags($._config.grafanaPrometheus.tags)
      + dashboard.withUid('cb079f93-fde4-41f0-862b-d4301d7c1c56')
      + dashboard.timepicker.withRefreshIntervals($._config.grafanaPrometheus.refresh)
      + dashboard.withVariables([
        datasourceVariable,
        clusterVariable,
        instanceVariable,
        urlVariable,
      ])
      + dashboard.withPanels(
        grafana.util.grid.makeGrid([
          row.new('Timestamps')
          + row.withPanels([
            timestampComparison,
            timestampComparisonRate,
          ]),
        ], panelWidth=12, panelHeight=7)
        +
        grafana.util.grid.makeGrid([
          row.new('Samples')
          + row.withPanels([
            samplesRate
            + panel.timeSeries.gridPos.withW(24),
          ]),
          row.new('Shards'),
        ], panelWidth=24, panelHeight=7, startY=8)
        +
        grafana.util.grid.wrapPanels([
          currentShards
          + panel.timeSeries.gridPos.withW(24),
          maxShards,
          minShards,
          desiredShards,
        ], panelWidth=8, panelHeight=7, startY=16)
        +
        grafana.util.grid.makeGrid([
          row.new('Shard Details')
          + row.withPanels([
            shardsCapacity,
            pendingSamples,
          ]),
          row.new('Segments')
          + row.withPanels([
            walSegment,
            queueSegment,
          ]),
        ], panelWidth=12, panelHeight=7, startY=24)
        +
        grafana.util.grid.makeGrid([
          row.new('Misc. Rates')
          + row.withPanels([
            droppedSamples,
            failedSamples,
            retriedSamples,
            enqueueRetries,
          ]),
        ], panelWidth=6, panelHeight=7, startY=40)
      ),
  },
}
