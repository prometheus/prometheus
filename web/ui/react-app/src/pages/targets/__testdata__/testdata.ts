/* eslint @typescript-eslint/camelcase: 0 */

import { ScrapePools } from '../target';

export const targetGroups: ScrapePools = Object.freeze({
  blackbox: {
    upCount: 2,
    targets: [
      {
        discoveredLabels: {
          __address__: 'http://prometheus.io',
          __metrics_path__: '/probe',
          __param_module: 'http_2xx',
          __scheme__: 'http',
          job: 'blackbox',
        },
        labels: {
          instance: 'http://prometheus.io',
          job: 'blackbox',
        },
        scrapePool: 'blackbox',
        scrapeUrl: 'http://127.0.0.1:9115/probe?module=http_2xx&target=http%3A%2F%2Fprometheus.io',
        globalUrl: 'http://localhost.localdomain:9115/probe?module=http_2xx&target=http%3A%2F%2Fprometheus.io',
        lastError: '',
        lastScrape: '2019-11-04T11:52:14.759299-07:00',
        lastScrapeDuration: 36560147,
        health: 'up',
      },
      {
        discoveredLabels: {
          __address__: 'https://prometheus.io',
          __metrics_path__: '/probe',
          __param_module: 'http_2xx',
          __scheme__: 'http',
          job: 'blackbox',
        },
        labels: {
          instance: 'https://prometheus.io',
          job: 'blackbox',
        },
        scrapePool: 'blackbox',
        scrapeUrl: 'http://127.0.0.1:9115/probe?module=http_2xx&target=https%3A%2F%2Fprometheus.io',
        globalUrl: 'http://localhost.localdomain:9115/probe?module=http_2xx&target=https%3A%2F%2Fprometheus.io',
        lastError: '',
        lastScrape: '2019-11-04T11:52:24.731096-07:00',
        lastScrapeDuration: 49448763,
        health: 'up',
      },
      {
        discoveredLabels: {
          __address__: 'http://example.com:8080',
          __metrics_path__: '/probe',
          __param_module: 'http_2xx',
          __scheme__: 'http',
          job: 'blackbox',
        },
        labels: {
          instance: 'http://example.com:8080',
          job: 'blackbox',
        },
        scrapePool: 'blackbox',
        scrapeUrl: 'http://127.0.0.1:9115/probe?module=http_2xx&target=http%3A%2F%2Fexample.com%3A8080',
        globalUrl: 'http://localhost.localdomain:9115/probe?module=http_2xx&target=http%3A%2F%2Fexample.com%3A8080',
        lastError: '',
        lastScrape: '2019-11-04T11:52:13.516654-07:00',
        lastScrapeDuration: 120916592,
        health: 'down',
      },
    ],
  },
  node_exporter: {
    upCount: 1,
    targets: [
      {
        discoveredLabels: {
          __address__: 'localhost:9100',
          __metrics_path__: '/metrics',
          __scheme__: 'http',
          job: 'node_exporter',
        },
        labels: {
          instance: 'localhost:9100',
          job: 'node_exporter',
        },
        scrapePool: 'node_exporter',
        scrapeUrl: 'http://localhost:9100/metrics',
        globalUrl: 'http://localhost.localdomain:9100/metrics',
        lastError: '',
        lastScrape: '2019-11-04T11:52:14.145703-07:00',
        lastScrapeDuration: 3842307,
        health: 'up',
      },
    ],
  },
  prometheus: {
    upCount: 1,
    targets: [
      {
        discoveredLabels: {
          __address__: 'localhost:9090',
          __metrics_path__: '/metrics',
          __scheme__: 'http',
          job: 'prometheus',
        },
        labels: {
          instance: 'localhost:9090',
          job: 'prometheus',
        },
        scrapePool: 'prometheus',
        scrapeUrl: 'http://localhost:9090/metrics',
        globalUrl: 'http://localhost.localdomain:9000/metrics',
        lastError: '',
        lastScrape: '2019-11-04T11:52:18.479731-07:00',
        lastScrapeDuration: 4050976,
        health: 'up',
      },
    ],
  },
});

export const sampleApiResponse = Object.freeze({
  status: 'success',
  data: {
    activeTargets: [
      {
        discoveredLabels: {
          __address__: 'http://prometheus.io',
          __metrics_path__: '/probe',
          __param_module: 'http_2xx',
          __scheme__: 'http',
          job: 'blackbox',
        },
        labels: {
          instance: 'http://prometheus.io',
          job: 'blackbox',
        },
        scrapePool: 'blackbox',
        scrapeUrl: 'http://127.0.0.1:9115/probe?module=http_2xx&target=http%3A%2F%2Fprometheus.io',
        lastError: '',
        lastScrape: '2019-11-04T11:52:14.759299-07:00',
        lastScrapeDuration: 36560147,
        health: 'up',
      },
      {
        discoveredLabels: {
          __address__: 'https://prometheus.io',
          __metrics_path__: '/probe',
          __param_module: 'http_2xx',
          __scheme__: 'http',
          job: 'blackbox',
        },
        labels: {
          instance: 'https://prometheus.io',
          job: 'blackbox',
        },
        scrapePool: 'blackbox',
        scrapeUrl: 'http://127.0.0.1:9115/probe?module=http_2xx&target=https%3A%2F%2Fprometheus.io',
        lastError: '',
        lastScrape: '2019-11-04T11:52:24.731096-07:00',
        lastScrapeDuration: 49448763,
        health: 'up',
      },
      {
        discoveredLabels: {
          __address__: 'http://example.com:8080',
          __metrics_path__: '/probe',
          __param_module: 'http_2xx',
          __scheme__: 'http',
          job: 'blackbox',
        },
        labels: {
          instance: 'http://example.com:8080',
          job: 'blackbox',
        },
        scrapePool: 'blackbox',
        scrapeUrl: 'http://127.0.0.1:9115/probe?module=http_2xx&target=http%3A%2F%2Fexample.com%3A8080',
        lastError: '',
        lastScrape: '2019-11-04T11:52:13.516654-07:00',
        lastScrapeDuration: 120916592,
        health: 'up',
      },
      {
        discoveredLabels: {
          __address__: 'localhost:9100',
          __metrics_path__: '/metrics',
          __scheme__: 'http',
          job: 'node_exporter',
        },
        labels: {
          instance: 'localhost:9100',
          job: 'node_exporter',
        },
        scrapePool: 'node_exporter',
        scrapeUrl: 'http://localhost:9100/metrics',
        lastError: '',
        lastScrape: '2019-11-04T11:52:14.145703-07:00',
        lastScrapeDuration: 3842307,
        health: 'up',
      },
      {
        discoveredLabels: {
          __address__: 'localhost:9090',
          __metrics_path__: '/metrics',
          __scheme__: 'http',
          job: 'prometheus/test',
        },
        labels: {
          instance: 'localhost:9090',
          job: 'prometheus/test',
        },
        scrapePool: 'prometheus/test',
        scrapeUrl: 'http://localhost:9090/metrics',
        lastError: '',
        lastScrape: '2019-11-04T11:52:18.479731-07:00',
        lastScrapeDuration: 4050976,
        health: 'up',
      },
    ],
  },
});
