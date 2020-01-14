import React from 'react';
import { mount, shallow } from 'enzyme';
import { targetGroups } from './__testdata__/testdata';
import ScrapePoolPanel, { columns } from './ScrapePoolPanel';
import { Button, Collapse, Table, Badge } from 'reactstrap';
import { Target, getColor } from './target';
import EndpointLink from './EndpointLink';
import TargetLabels from './TargetLabels';

describe('ScrapePoolPanel', () => {
  const defaultProps = {
    scrapePool: 'blackbox',
    targetGroup: targetGroups.blackbox,
  };
  const scrapePoolPanel = shallow(<ScrapePoolPanel {...defaultProps} />);

  it('renders a container', () => {
    const div = scrapePoolPanel.find('div').filterWhere(elem => elem.hasClass('container'));
    expect(div).toHaveLength(1);
  });

  describe('Header', () => {
    it('renders an anchor with up count and danger color if upCount < targetsCount', () => {
      const anchor = scrapePoolPanel.find('a');
      expect(anchor).toHaveLength(1);
      expect(anchor.prop('id')).toEqual('pool-blackbox');
      expect(anchor.prop('href')).toEqual('#pool-blackbox');
      expect(anchor.text()).toEqual('blackbox (2/3 up)');
      expect(anchor.prop('className')).toEqual('danger');
    });

    it('renders an anchor with up count and normal color if upCount == targetsCount', () => {
      const props = {
        scrapePool: 'prometheus',
        targetGroup: targetGroups.prometheus,
      };
      const scrapePoolPanel = shallow(<ScrapePoolPanel {...props} />);
      const anchor = scrapePoolPanel.find('a');
      expect(anchor).toHaveLength(1);
      expect(anchor.prop('id')).toEqual('pool-prometheus');
      expect(anchor.prop('href')).toEqual('#pool-prometheus');
      expect(anchor.text()).toEqual('prometheus (1/1 up)');
      expect(anchor.prop('className')).toEqual('normal');
    });

    it('renders a show more btn if collapsed', () => {
      const props = {
        scrapePool: 'prometheus',
        targetGroup: targetGroups.prometheus,
      };
      const div = document.createElement('div');
      div.id = `series-labels-prometheus-0`;
      document.body.appendChild(div);
      const scrapePoolPanel = mount(<ScrapePoolPanel {...props} />);

      const btn = scrapePoolPanel.find(Button);
      btn.simulate('click');
      const collapse = scrapePoolPanel.find(Collapse);
      expect(collapse.prop('isOpen')).toBe(false);
    });
  });

  it('renders a Collapse component', () => {
    const collapse = scrapePoolPanel.find(Collapse);
    expect(collapse.prop('isOpen')).toBe(true);
  });

  describe('Table', () => {
    it('renders a table', () => {
      const table = scrapePoolPanel.find(Table);
      const headers = table.find('th');
      expect(table).toHaveLength(1);
      expect(headers).toHaveLength(6);
      columns.forEach(col => {
        expect(headers.contains(col));
      });
    });

    describe('for each target', () => {
      const table = scrapePoolPanel.find(Table);
      defaultProps.targetGroup.targets.forEach(
        ({ discoveredLabels, labels, scrapeUrl, lastError, health }: Target, idx: number) => {
          const row = table.find('tr').at(idx + 1);

          it('renders an EndpointLink with the scrapeUrl', () => {
            const link = row.find(EndpointLink);
            expect(link).toHaveLength(1);
            expect(link.prop('endpoint')).toEqual(scrapeUrl);
          });

          it('renders a badge for health', () => {
            const td = row.find('td').filterWhere(elem => Boolean(elem.hasClass('state')));
            const badge = td.find(Badge);
            expect(badge).toHaveLength(1);
            expect(badge.prop('color')).toEqual(getColor(health));
            expect(badge.children().text()).toEqual(health.toUpperCase());
          });

          it('renders series labels', () => {
            const targetLabels = row.find(TargetLabels);
            expect(targetLabels).toHaveLength(1);
            expect(targetLabels.prop('discoveredLabels')).toEqual(discoveredLabels);
            expect(targetLabels.prop('labels')).toEqual(labels);
          });

          it('renders last scrape time', () => {
            const lastScrapeCell = row.find('td').filterWhere(elem => Boolean(elem.hasClass('last-scrape')));
            expect(lastScrapeCell).toHaveLength(1);
          });

          it('renders last scrape duration', () => {
            const lastScrapeCell = row.find('td').filterWhere(elem => Boolean(elem.hasClass('scrape-duration')));
            expect(lastScrapeCell).toHaveLength(1);
          });

          it('renders a badge for Errors', () => {
            const td = row.find('td').filterWhere(elem => Boolean(elem.hasClass('errors')));
            const badge = td.find(Badge);
            expect(badge).toHaveLength(lastError ? 1 : 0);
            if (lastError) {
              expect(badge.prop('color')).toEqual('danger');
              expect(badge.children().text()).toEqual(lastError);
            }
          });
        }
      );
    });
  });
});
