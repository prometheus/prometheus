local g = import 'grafana-builder/grafana.libsonnet';

{
  _config+:: {
    storage_backend: error 'must specify storage backend (cassandra, gcp)',
  },

  dashboards+: {
    'prometheus.json':
      g.dashboard('Prometheus')
      .addMultiTemplate('job', 'prometheus_build_info', 'job')
      .addMultiTemplate('instance', 'prometheus_build_info', 'instance')
      # Prometheus is quite commonly configured with honor_labels set to true;
      # therefor job and instance is not the prometheus server in many queries!.
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
            verstion: { alias: 'Version' },
            'Value #A': { alias: 'Count', type: 'hidden' },
            'Value #B': { alias: 'Uptime' },
          })
        )
      )
      .addRow(
        g.row('Discovery')
        .addPanel(
          g.panel('Target Sync') +
          g.queryPanel('sum(rate(prometheus_target_sync_length_seconds_sum{job=~"$job",instance=~"$instance"}[2m])) by (scrape_job) * 1e3', '{{scrape_job}}') +
          { yaxes: g.yaxes('ms') }
        )
        .addPanel(
          g.panel('Targets') +
          g.queryPanel('count(up{})', 'Targets') +
          g.stack
        )
      )
      .addRow(
        g.row('Retrieval')
        .addPanel(
          g.panel('Target Scrape Duration') +
          g.queryPanel('1e3 * sum(scrape_duration_seconds) / count(scrape_duration_seconds)', 'Average') +
          { yaxes: g.yaxes('ms') }
        )
        .addPanel(
          g.panel('Scrape failures') +
          g.queryPanel([
            'sum by (job) (rate(prometheus_target_scrapes_exceeded_sample_limit_total[1m]))',
            'sum by (job) (rate(prometheus_target_scrapes_sample_duplicate_timestamp_total[1m]))',
            'sum by (job) (rate(prometheus_target_scrapes_sample_out_of_bounds_total[1m]))',
            'sum by (job) (rate(prometheus_target_scrapes_sample_out_of_order_total[1m]))',
          ], [
            'exceeded sample limit: {{job}}',
            'duplicate timestamp: {{job}}',
            'out of bounds: {{job}}',
            'out of order: {{job}}',
          ]) +
          g.stack
        )
        .addPanel(
          g.panel('Appended Samples') +
          g.queryPanel('rate(prometheus_tsdb_head_samples_appended_total{job=~"$job",instance=~"$instance"}[1m])', '{{job}} {{instance}}') +
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
          g.queryPanel('rate(prometheus_engine_query_duration_seconds_count{job=~"$job",instance=~"$instance",slice="inner_eval"}[1m])', '{{job}} {{instance}}') +
          g.stack,
        )
        .addPanel(
          g.panel('Stage Duration') +
          g.queryPanel('max by (slice) (prometheus_engine_query_duration_seconds{quantile="0.9",job=~"$job",instance=~"$instance"}) * 1e3', '{{slice}}') +
          { yaxes: g.yaxes('ms') } +
          g.stack,
        )
      )
  },
}
