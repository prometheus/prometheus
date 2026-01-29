import { describe, it, expect } from 'vitest';
import { buildRulesPageData } from './rulesPageUtils';
import { Rule, RulesResult } from '../api/responseTypes/rules';

// Mock data
const mockRules: Rule[] = [
    {
        name: 'test_alert',
        query: 'up == 0',
        type: 'alerting',
        health: 'ok',
        labels: { severity: 'critical', app: 'backend' },
        annotations: {},
        duration: 0,
        keepFiringFor: 0,
        state: 'firing',
        alerts: [],
        evaluationTime: '0.001',
        lastEvaluation: '2023-01-01T00:00:00Z',
    },
    {
        name: 'test_record',
        query: 'rate(http_requests_total[5m])',
        type: 'recording',
        health: 'ok',
        labels: { app: 'frontend' },
        evaluationTime: '0.002',
        lastEvaluation: '2023-01-01T00:00:00Z',
    },
];

const mockData: RulesResult = {
    groups: [
        {
            name: 'group_one',
            file: 'rules.yaml',
            interval: '60s',
            evaluationTime: '0.003',
            lastEvaluation: '2023-01-01T00:00:00Z',
            rules: [mockRules[0]],
        },
        {
            name: 'group_two',
            file: 'rules.yaml',
            interval: '60s',
            evaluationTime: '0.003',
            lastEvaluation: '2023-01-01T00:00:00Z',
            rules: [mockRules[1]],
        },
    ],
};

describe('buildRulesPageData', () => {
    it('should return all rules when search is empty', () => {
        const result = buildRulesPageData(mockData, '', []);
        expect(result.groups).toHaveLength(2);
        expect(result.groups[0].rules).toHaveLength(1);
        expect(result.groups[1].rules).toHaveLength(1);
    });

    it('should filter by rule name', () => {
        const result = buildRulesPageData(mockData, 'test_alert', []);
        expect(result.groups[0].rules).toHaveLength(1);
        expect(result.groups[1].rules).toHaveLength(0);
    });

    it('should filter by label', () => {
        const result = buildRulesPageData(mockData, 'backend', []);
        expect(result.groups[0].rules).toHaveLength(1);
        expect(result.groups[1].rules).toHaveLength(0);
    });

    it('should filter by group name', () => {
        const result = buildRulesPageData(mockData, 'group_two', []);
        expect(result.groups[0].rules).toHaveLength(0);
        expect(result.groups[1].rules).toHaveLength(1);
    });

    it('should filter by query expression', () => {
        const result = buildRulesPageData(mockData, 'rate(http_requests', []);
        expect(result.groups[0].rules).toHaveLength(0);
        expect(result.groups[1].rules).toHaveLength(1);
    });

    it('should filter by health state', () => {
        // Both are OK
        const result = buildRulesPageData(mockData, '', ['ok']);
        expect(result.groups[0].rules).toHaveLength(1);
        expect(result.groups[1].rules).toHaveLength(1);

        const resultErr = buildRulesPageData(mockData, '', ['err']);
        expect(resultErr.groups[0].rules).toHaveLength(0);
        expect(resultErr.groups[1].rules).toHaveLength(0);
    });

    it('should combine search and health filter', () => {
        const result = buildRulesPageData(mockData, 'test', ['ok']);
        expect(result.groups[0].rules).toHaveLength(1);
        expect(result.groups[1].rules).toHaveLength(1);
    });
});
