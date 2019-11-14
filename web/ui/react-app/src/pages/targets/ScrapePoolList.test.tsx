import * as React from 'react';
import { mount, shallow, ReactWrapper } from 'enzyme';
import { act } from 'react-dom/test-utils';
import { Alert } from 'reactstrap';
import { sampleApiResponse } from './__testdata__/testdata';
import ScrapePoolList from './ScrapePoolList';
import ScrapePoolPanel from './ScrapePoolPanel';
import { Target } from './target';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';
import { FetchMock } from 'jest-fetch-mock/types';

describe('Flags', () => {
  const defaultProps = {
    filter: { showHealthy: true, showUnhealthy: true },
    pathPrefix: '..',
  };

  beforeEach(() => {
    fetchMock.resetMocks();
  });

  describe('before data is returned', () => {
    const scrapePoolList = shallow(<ScrapePoolList {...defaultProps} />);
    const spinner = scrapePoolList.find(FontAwesomeIcon);

    it('renders a spinner', () => {
      expect(spinner.prop('icon')).toEqual(faSpinner);
      expect(spinner.prop('spin')).toBe(true);
    });

    it('renders exactly one spinner', () => {
      expect(spinner).toHaveLength(1);
    });
  });

  describe('when data is returned', () => {
    let scrapePoolList: ReactWrapper;
    let mock: FetchMock;
    beforeEach(() => {
      //Tooltip requires DOM elements to exist. They do not in enzyme rendering so we must manually create them.
      const scrapePools: { [key: string]: number } = { blackbox: 3, node_exporter: 1, prometheus: 1 };
      Object.keys(scrapePools).forEach((pool: string): void => {
        Array.from(Array(scrapePools[pool]).keys()).forEach((idx: number): void => {
          const div = document.createElement('div');
          div.id = `series-labels-${pool}-${idx}`;
          document.body.appendChild(div);
        });
      });
      mock = fetchMock.mockResponse(JSON.stringify(sampleApiResponse));
    });

    it('renders a table', async () => {
      await act(async () => {
        scrapePoolList = mount(<ScrapePoolList {...defaultProps} />);
      });
      scrapePoolList.update();
      expect(mock).toHaveBeenCalledWith('../api/v1/targets?state=active', undefined);
      const panels = scrapePoolList.find(ScrapePoolPanel);
      expect(panels).toHaveLength(3);
      const activeTargets: Target[] = sampleApiResponse.data.activeTargets as Target[];
      activeTargets.forEach(({ scrapePool }: Target) => {
        const panel = scrapePoolList.find(ScrapePoolPanel).filterWhere(panel => panel.prop('scrapePool') === scrapePool);
        expect(panel).toHaveLength(1);
      });
    });

    it('filters by health', async () => {
      const props = {
        ...defaultProps,
        filter: { showHealthy: false, showUnhealthy: true },
      };
      await act(async () => {
        scrapePoolList = mount(<ScrapePoolList {...props} />);
      });
      scrapePoolList.update();
      expect(mock).toHaveBeenCalledWith('../api/v1/targets?state=active', undefined);
      const panels = scrapePoolList.find(ScrapePoolPanel);
      expect(panels).toHaveLength(0);
    });
  });

  describe('when an error is returned', () => {
    it('displays an alert', async () => {
      const mock = fetchMock.mockReject(new Error('Error fetching targets'));

      let scrapePoolList: any;
      await act(async () => {
        scrapePoolList = mount(<ScrapePoolList {...defaultProps} />);
      });
      scrapePoolList.update();

      expect(mock).toHaveBeenCalledWith('../api/v1/targets?state=active', undefined);
      const alert = scrapePoolList.find(Alert);
      expect(alert.prop('color')).toBe('danger');
      expect(alert.text()).toContain('Error fetching targets');
    });
  });
});
