local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local g = import 'github.com/grafana/jsonnet-libs/grafana-builder/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local singlestat = grafana.singlestat;
local prometheus = grafana.prometheus;
local graphPanel = grafana.graphPanel;
local tablePanel = grafana.tablePanel;
local template = grafana.template;
{
  grafanaDashboards+:: {
    'prometheus.json':
      g.dashboard(
        '%(prefix)sOverview' % $._config.grafanaPrometheus
      )
      .addMultiTemplate('job', 'prometheus_build_info{%(prometheusSelector)s}' % $._config, 'job')
      .addMultiTemplate('instance', 'prometheus_build_info{job=~"$job"}', 'instance')
      .addRow(
        g.row('Prometheus Stats')
        .addPanel(
          g.panel('Prometheus Stats') +
          g.tablePanel([
            'count by (job, instance, version) (prometheus_build_info{job=~"$job", instance=~"$instance"})',
            'max by (job, instance) (time() - process_start_time_seconds{job=~"$job", instance=~"$instance"})',
          ], {
            job: { alias: 'Job' },
            instance: { alias: 'Instance' },
            version: { alias: 'Version' },
            'Value #A': { alias: 'Count', type: 'hidden' },
            'Value #B': { alias: 'Uptime', type: 'number', unit: 's' },
          })
        )
      )
      .addRow(
        g.row('Discovery')
        .addPanel(
          g.panel('Target Sync') +
          g.queryPanel('sum(rate(prometheus_target_sync_length_seconds_sum{job=~"$job",instance=~"$instance"}[5m])) by (scrape_job) * 1e3', '{{scrape_job}}') +
          { yaxes: g.yaxes('ms') }
        )
        .addPanel(
          g.panel('Targets') +
          g.queryPanel('sum(prometheus_sd_discovered_targets{job=~"$job",instance=~"$instance"})', 'Targets') +
          g.stack
        )
      )
      .addRow(
        g.row('Retrieval')
        .addPanel(
          g.panel('Average Scrape Interval Duration') +
          g.queryPanel('rate(prometheus_target_interval_length_seconds_sum{job=~"$job",instance=~"$instance"}[5m]) / rate(prometheus_target_interval_length_seconds_count{job=~"$job",instance=~"$instance"}[5m]) * 1e3', '{{interval}} configured') +
          { yaxes: g.yaxes('ms') }
        )
        .addPanel(
          g.panel('Scrape failures') +
          g.queryPanel([
            'sum by (job) (rate(prometheus_target_scrapes_exceeded_body_size_limit_total[1m]))',
            'sum by (job) (rate(prometheus_target_scrapes_exceeded_sample_limit_total[1m]))',
            'sum by (job) (rate(prometheus_target_scrapes_sample_duplicate_timestamp_total[1m]))',
            'sum by (job) (rate(prometheus_target_scrapes_sample_out_of_bounds_total[1m]))',
            'sum by (job) (rate(prometheus_target_scrapes_sample_out_of_order_total[1m]))',
          ], [
            'exceeded body size limit: {{job}}',
            'exceeded sample limit: {{job}}',
            'duplicate timestamp: {{job}}',
            'out of bounds: {{job}}',
            'out of order: {{job}}',
          ]) +
          g.stack
        )
        .addPanel(
          g.panel('Appended Samples') +
          g.queryPanel('rate(prometheus_tsdb_head_samples_appended_total{job=~"$job",instance=~"$instance"}[5m])', '{{job}} {{instance}}') +
          g.stack
        )
      )
      .addRow(
        g.row('Storage')
        .addPanel(
          g.panel('Head Series') +
          g.queryPanel('prometheus_tsdb_head_series{job=~"$job",instance=~"$instance"}', '{{job}} {{instance}} head series') +
          g.stack
        )
        .addPanel(
          g.panel('Head Chunks') +
          g.queryPanel('prometheus_tsdb_head_chunks{job=~"$job",instance=~"$instance"}', '{{job}} {{instance}} head chunks') +
          g.stack
        )
      )
      .addRow(
        g.row('Query')
        .addPanel(
          g.panel('Query Rate') +
          g.queryPanel('rate(prometheus_engine_query_duration_seconds_count{job=~"$job",instance=~"$instance",slice="inner_eval"}[5m])', '{{job}} {{instance}}') +
          g.stack,
        )
        .addPanel(
          g.panel('Stage Duration') +
          g.queryPanel('max by (slice) (prometheus_engine_query_duration_seconds{quantile="0.9",job=~"$job",instance=~"$instance"}) * 1e3', '{{slice}}') +
          { yaxes: g.yaxes('ms') } +
          g.stack,
        )
      ) + {
        tags: $._config.grafanaPrometheus.tags,
        refresh: $._config.grafanaPrometheus.refresh,
      },
    // Remote write specific dashboard.
    'prometheus-remote-write.json':
      local timestampComparison =
        graphPanel.new(
          'Highest Timestamp In vs. Highest Timestamp Sent',
          datasource='$datasource',
          span=6,
        )
        .addTarget(prometheus.target(
          |||
            (
              prometheus_remote_storage_highest_timestamp_in_seconds{cluster=~"$cluster", instance=~"$instance"} 
            -  
              ignoring(remote_name, url) group_right(instance) (prometheus_remote_storage_queue_highest_sent_timestamp_seconds{cluster=~"$cluster", instance=~"$instance"} != 0)
            )
          |||,
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}',
        ));

      local timestampComparisonRate =
        graphPanel.new(
          'Rate[5m]',
          datasource='$datasource',
          span=6,
        )
        .addTarget(prometheus.target(
          |||
            clamp_min(
              rate(prometheus_remote_storage_highest_timestamp_in_seconds{cluster=~"$cluster", instance=~"$instance"}[5m])  
            - 
              ignoring (remote_name, url) group_right(instance) rate(prometheus_remote_storage_queue_highest_sent_timestamp_seconds{cluster=~"$cluster", instance=~"$instance"}[5m])
            , 0)
          |||,
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}',
        ));

      local samplesRate =
        graphPanel.new(
          'Rate, in vs. succeeded or dropped [5m]',
          datasource='$datasource',
          span=12,
        )
        .addTarget(prometheus.target(
          |||
            rate(
              prometheus_remote_storage_samples_in_total{cluster=~"$cluster", instance=~"$instance"}[5m])
            - 
              ignoring(remote_name, url) group_right(instance) (rate(prometheus_remote_storage_succeeded_samples_total{cluster=~"$cluster", instance=~"$instance"}[5m]) or rate(prometheus_remote_storage_samples_total{cluster=~"$cluster", instance=~"$instance"}[5m]))
            - 
              (rate(prometheus_remote_storage_dropped_samples_total{cluster=~"$cluster", instance=~"$instance"}[5m]) or rate(prometheus_remote_storage_samples_dropped_total{cluster=~"$cluster", instance=~"$instance"}[5m]))
          |||,
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}'
        ));

      local currentShards =
        graphPanel.new(
          'Current Shards',
          datasource='$datasource',
          span=12,
          min_span=6,
        )
        .addTarget(prometheus.target(
          'prometheus_remote_storage_shards{cluster=~"$cluster", instance=~"$instance"}',
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}'
        ));

      local maxShards =
        graphPanel.new(
          'Max Shards',
          datasource='$datasource',
          span=4,
        )
        .addTarget(prometheus.target(
          'prometheus_remote_storage_shards_max{cluster=~"$cluster", instance=~"$instance"}',
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}'
        ));

      local minShards =
        graphPanel.new(
          'Min Shards',
          datasource='$datasource',
          span=4,
        )
        .addTarget(prometheus.target(
          'prometheus_remote_storage_shards_min{cluster=~"$cluster", instance=~"$instance"}',
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}'
        ));

      local desiredShards =
        graphPanel.new(
          'Desired Shards',
          datasource='$datasource',
          span=4,
        )
        .addTarget(prometheus.target(
          'prometheus_remote_storage_shards_desired{cluster=~"$cluster", instance=~"$instance"}',
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}'
        ));

      local shardsCapacity =
        graphPanel.new(
          'Shard Capacity',
          datasource='$datasource',
          span=6,
        )
        .addTarget(prometheus.target(
          'prometheus_remote_storage_shard_capacity{cluster=~"$cluster", instance=~"$instance"}',
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}'
        ));


      local pendingSamples =
        graphPanel.new(
          'Pending Samples',
          datasource='$datasource',
          span=6,
        )
        .addTarget(prometheus.target(
          'prometheus_remote_storage_pending_samples{cluster=~"$cluster", instance=~"$instance"} or prometheus_remote_storage_samples_pending{cluster=~"$cluster", instance=~"$instance"}',
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}'
        ));

      local walSegment =
        graphPanel.new(
          'TSDB Current Segment',
          datasource='$datasource',
          span=6,
          formatY1='none',
        )
        .addTarget(prometheus.target(
          'prometheus_tsdb_wal_segment_current{cluster=~"$cluster", instance=~"$instance"}',
          legendFormat='{{cluster}}:{{instance}}'
        ));

      local queueSegment =
        graphPanel.new(
          'Remote Write Current Segment',
          datasource='$datasource',
          span=6,
          formatY1='none',
        )
        .addTarget(prometheus.target(
          'prometheus_wal_watcher_current_segment{cluster=~"$cluster", instance=~"$instance"}',
          legendFormat='{{cluster}}:{{instance}} {{consumer}}'
        ));

      local droppedSamples =
        graphPanel.new(
          'Dropped Samples',
          datasource='$datasource',
          span=3,
        )
        .addTarget(prometheus.target(
          'rate(prometheus_remote_storage_dropped_samples_total{cluster=~"$cluster", instance=~"$instance"}[5m]) or rate(prometheus_remote_storage_samples_dropped_total{cluster=~"$cluster", instance=~"$instance"}[5m])',
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}'
        ));

      local failedSamples =
        graphPanel.new(
          'Failed Samples',
          datasource='$datasource',
          span=3,
        )
        .addTarget(prometheus.target(
          'rate(prometheus_remote_storage_failed_samples_total{cluster=~"$cluster", instance=~"$instance"}[5m]) or rate(prometheus_remote_storage_samples_failed_total{cluster=~"$cluster", instance=~"$instance"}[5m])',
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}'
        ));

      local retriedSamples =
        graphPanel.new(
          'Retried Samples',
          datasource='$datasource',
          span=3,
        )
        .addTarget(prometheus.target(
          'rate(prometheus_remote_storage_retried_samples_total{cluster=~"$cluster", instance=~"$instance"}[5m]) or rate(prometheus_remote_storage_samples_retried_total{cluster=~"$cluster", instance=~"$instance"}[5m])',
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}'
        ));

      local enqueueRetries =
        graphPanel.new(
          'Enqueue Retries',
          datasource='$datasource',
          span=3,
        )
        .addTarget(prometheus.target(
          'rate(prometheus_remote_storage_enqueue_retries_total{cluster=~"$cluster", instance=~"$instance"}[5m])',
          legendFormat='{{cluster}}:{{instance}} {{remote_name}}:{{url}}'
        ));

      dashboard.new(
        title='%(prefix)sRemote Write' % $._config.grafanaPrometheus,
        editable=true
      )
      .addTemplate(
        {
          hide: 0,
          label: null,
          name: 'datasource',
          options: [],
          query: 'prometheus',
          refresh: 1,
          regex: '',
          type: 'datasource',
        },
      )
      .addTemplate(
        template.new(
          'cluster',
          '$datasource',
          'label_values(kube_pod_container_info{image=~".*prometheus.*"}, cluster)' % $._config,
          refresh='time',
          current={
            selected: true,
            text: 'All',
            value: '$__all',
          },
          includeAll=true,
        )
      )
      .addTemplate(
        template.new(
          'instance',
          '$datasource',
          'label_values(prometheus_build_info{cluster=~"$cluster"}, instance)' % $._config,
          refresh='time',
          current={
            selected: true,
            text: 'All',
            value: '$__all',
          },
          includeAll=true,
        )
      )
      .addTemplate(
        template.new(
          'url',
          '$datasource',
          'label_values(prometheus_remote_storage_shards{cluster=~"$cluster", instance=~"$instance"}, url)' % $._config,
          refresh='time',
          includeAll=true,
        )
      )
      .addRow(
        row.new('Timestamps')
        .addPanel(timestampComparison)
        .addPanel(timestampComparisonRate)
      )
      .addRow(
        row.new('Samples')
        .addPanel(samplesRate)
      )
      .addRow(
        row.new(
          'Shards'
        )
        .addPanel(currentShards)
        .addPanel(maxShards)
        .addPanel(minShards)
        .addPanel(desiredShards)
      )
      .addRow(
        row.new('Shard Details')
        .addPanel(shardsCapacity)
        .addPanel(pendingSamples)
      )
      .addRow(
        row.new('Segments')
        .addPanel(walSegment)
        .addPanel(queueSegment)
      )
      .addRow(
        row.new('Misc. Rates')
        .addPanel(droppedSamples)
        .addPanel(failedSamples)
        .addPanel(retriedSamples)
        .addPanel(enqueueRetries)
      ) + {
        tags: $._config.grafanaPrometheus.tags,
        refresh: $._config.grafanaPrometheus.refresh,
      },
  },
}
