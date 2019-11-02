/* eslint @typescript-eslint/camelcase: 0 */

import { sampleApiResponse } from './__testdata__/testdata';
import { groupTargets, Target, TargetGroups } from './target';

describe('groupTargets', () => {
  const targets: Target[] = sampleApiResponse.data.activeTargets;
  const targetGroups: TargetGroups = groupTargets(targets);

  it('groups a list of targets by scrape job', () => {
    ['blackbox', 'prometheus', 'node_exporter'].forEach(scrapeJob => {
      expect(Object.keys(targetGroups)).toContain(scrapeJob);
    });
    Object.keys(targetGroups).forEach((scrapeJob: string): void => {
      const ts: Target[] = targetGroups[scrapeJob].targets;
      ts.forEach((t: Target) => {
        expect(t.scrapeJob).toEqual(scrapeJob);
      });
    });
  });

  it('adds up metadata during aggregation', () => {
    const testCases: { [key: string]: number } = { blackbox: 3, prometheus: 1, node_exporter: 1 };
    Object.keys(testCases).forEach((scrapeJob: string): void => {
      expect(targetGroups[scrapeJob].metadata.up).toEqual(testCases[scrapeJob]);
    });
  });
});
