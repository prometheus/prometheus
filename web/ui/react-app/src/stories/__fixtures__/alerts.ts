import { RuleGroup } from '../../pages/alerts/AlertContents';

const alertRules: RuleGroup[] = [
  {
    name: 'Short name',
    file: 'rule01.yml',
    rules: [
      {
        alerts: [
          {
            activeAt: new Date().toISOString(),
            annotations: { summary: 'this is an annotation' },
            labels: { cluster: 'dev', severity: 'warning', instance: 'host1' },
            state: 'pending',
            value: '10',
          },
          {
            activeAt: new Date().toISOString(),
            annotations: { summary: 'this is an annotation' },
            labels: { cluster: 'dev', severity: 'warning', instance: 'host2' },
            state: 'pending',
            value: '20',
          },
        ],
        annotations: { summary: 'this is an annotation' },
        duration: 60,
        evaluationTime: '0.03',
        health: 'ok',
        labels: { cluster: 'dev', severity: 'warning' },
        lastError: '',
        lastEvaluation: new Date().toISOString(),
        name: 'Test Alert',
        query: 'up == 0',
        state: 'pending',
        type: 'alerting',
      },
    ],
    interval: 60,
  },
  {
    name: 'A_very_long_name' + 'e'.repeat(200),
    file: 'rule02.yml',
    rules: [
      {
        alerts: [
          {
            activeAt: new Date().toISOString(),
            annotations: { summary: 'this is an annotation' },
            labels: { cluster: 'dev', severity: 'warning', instance: 'localhost' },
            state: 'firing',
            value: '1',
          },
        ],
        annotations: { summary: 'this is an annotation' },
        duration: 60,
        evaluationTime: '0.03',
        health: 'ok',
        labels: { cluster: 'dev', severity: 'warning', instance: 'a_very_long_name' + 'e'.repeat(100) },
        lastError: '',
        lastEvaluation: new Date().toISOString(),
        name: 'Test Alert',
        query: 'up == 0',
        state: 'firing',
        type: 'alerting',
      },
    ],
    interval: 60,
  },
  {
    name: 'A_very_long_name' + 'e'.repeat(200),
    file: 'rule03.yml',
    rules: [
      {
        alerts: [
          {
            activeAt: new Date().toISOString(),
            annotations: { summary: 'A_very_long_annotation_' + '_1'.repeat(100) },
            labels: { cluster: 'dev', severity: 'warning', instance: 'host1' },
            state: 'inactive',
            value: '1',
          },
          {
            activeAt: new Date().toISOString(),
            annotations: { summary: 'Short annotation' },
            labels: { cluster: 'dev', severity: 'warning', instance: 'host2' },
            state: 'inactive',
            value: '1',
          },
        ],
        annotations: { summary: 'A_very_long_annotation_' + '_1'.repeat(100) },
        duration: 60,
        evaluationTime: '0.03',
        health: 'ok',
        labels: { cluster: 'dev', severity: 'warning' },
        lastError: '',
        lastEvaluation: new Date().toISOString(),
        name: 'A_very_long_name_' + '_1'.repeat(100),
        query: 'up == 0',
        state: 'inactive',
        type: 'alerting',
      },
    ],
    interval: 60,
  },
];

export default alertRules;
