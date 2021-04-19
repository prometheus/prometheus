import React from 'react';

import { Story, Meta } from '@storybook/react/types-6-0';

import { PanelListContent, PanelListContentProps } from '../pages/graph/PanelList';
import { PanelType } from '../pages/graph/Panel';

export default {
  title: 'PanelListContent',
  component: PanelListContent,
} as Meta;

const Template: Story<PanelListContentProps> = args => <PanelListContent {...args} />;

export const LegacyEditor = Template.bind({});
LegacyEditor.args = {
  panels: [
    {
      key: '1',
      id: '1',
      options: {
        expr: 'http_requests_total{job="web"}',
        type: PanelType.Graph,
        range: 3600 * 1000,
        endTime: null,
        resolution: null,
        stacked: false,
      },
    },
  ],
  metrics: [],
  useLocalTime: false,
  useExperimentalEditor: false,
  queryHistoryEnabled: false,
  enableAutocomplete: false,
  enableHighlighting: false,
  enableLinter: false,
};

export const CMEEditor = Template.bind({});
CMEEditor.args = {
  panels: [
    {
      key: '1',
      id: '1',
      options: {
        expr: 'http_requests_total{job="web"}',
        type: PanelType.Graph,
        range: 3600 * 1000,
        endTime: null,
        resolution: null,
        stacked: false,
      },
    },
  ],
  metrics: [],
  useLocalTime: false,
  useExperimentalEditor: true,
  queryHistoryEnabled: false,
  enableAutocomplete: false,
  enableHighlighting: true,
  enableLinter: true,
};

export const MultiplePanels = Template.bind({});
MultiplePanels.args = {
  panels: [
    {
      key: '1',
      id: '1',
      options: {
        expr: 'http_requests_total{job="web"}',
        type: PanelType.Graph,
        range: 3600 * 1000,
        endTime: null,
        resolution: null,
        stacked: false,
      },
    },
  ],
  metrics: [],
  useLocalTime: false,
  useExperimentalEditor: true,
  queryHistoryEnabled: false,
  enableAutocomplete: false,
  enableHighlighting: true,
  enableLinter: true,
};
