import * as React from 'react';
import { mount, shallow, ReactWrapper } from 'enzyme';
import { act } from 'react-dom/test-utils';
import { Alert } from 'reactstrap';
import Loader from '../../Loader';
import { sampleApiResponse } from './__testdata__/testdata';
import JobsList from './JobsList';
import JobPanel from './JobPanel';
import { Target } from './target';

describe('Flags', () => {
  const defaultProps = {
    filter: { showHealthy: true, showUnhealthy: true },
    pathPrefix: '..',
  };

  beforeEach(() => {
    fetch.resetMocks();
  });

  describe('before data is returned', () => {
    it('renders a Loader', () => {
      const jobsList = shallow(<JobsList {...defaultProps} />);
      expect(jobsList.find(Loader)).toHaveLength(1);
    });
  });

  describe('when data is returned', () => {
    let jobsList: ReactWrapper;
    let mock: Promise<Response>;
    beforeEach(() => {
      const div = document.createElement('div');
      div.id = 'series-labels';
      document.body.appendChild(div);
      mock = fetch.mockResponse(JSON.stringify(sampleApiResponse));
    });

    it('renders a table', async () => {
      await act(async () => {
        jobsList = mount(<JobsList {...defaultProps} />);
      });
      jobsList.update();
      expect(mock).toHaveBeenCalledWith('../api/v1/targets?state=active', undefined);
      const jobPanels = jobsList.find(JobPanel);
      expect(jobPanels).toHaveLength(3);
      const activeTargets: Target[] = sampleApiResponse.data.activeTargets;
      activeTargets.forEach(({ scrapeJob }: Target) => {
        const panel = jobsList.find(JobPanel).filterWhere(panel => panel.prop('scrapeJob') === scrapeJob);
        expect(panel).toHaveLength(1);
      });
    });

    it('filters by health', async () => {
      const props = {
        ...defaultProps,
        filter: { showHealthy: false, showUnhealthy: true },
      };
      await act(async () => {
        jobsList = mount(<JobsList {...props} />);
      });
      jobsList.update();
      expect(mock).toHaveBeenCalledWith('../api/v1/targets?state=active', undefined);
      const jobPanels = jobsList.find(JobPanel);
      expect(jobPanels).toHaveLength(0);
    });
  });

  describe('when an error is returned', () => {
    it('displays an alert', async () => {
      const mock = fetch.mockReject(new Error('Error fetching targets'));

      let jobsList: ReactWrapper;
      await act(async () => {
        jobsList = mount(<JobsList {...defaultProps} />);
      });
      jobsList.update();

      expect(mock).toHaveBeenCalledWith('../api/v1/targets?state=active', undefined);
      const alert = jobsList.find(Alert);
      expect(alert.prop('color')).toBe('danger');
      expect(alert.text()).toContain('Error fetching targets');
    });
  });
});
