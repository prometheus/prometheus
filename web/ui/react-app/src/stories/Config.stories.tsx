import React from 'react';

import { Story, Meta } from '@storybook/react/types-6-0';

import { ConfigContent, ConfigContentProps } from '../pages/config/Config';

export default {
  title: 'ConfigContent',
  component: ConfigContent,
} as Meta;

const Template: Story<ConfigContentProps> = args => <ConfigContent {...args} />;

export const LoadError = Template.bind({});
LoadError.args = {
  error: new Error('Mock error message'),
};

export const EmptyData = Template.bind({});
EmptyData.args = {
  data: {},
};

export const OneLineData = Template.bind({});
OneLineData.args = {
  data: { yaml: 'foo' },
};

export const MultiLineData = Template.bind({});
MultiLineData.args = {
  data: {
    yaml: `global:
  scrape_interval: 15s
  scrape_timeout: 10s
  evaluation_interval: 15s
  external_labels:
    env: dev
alerting:
  alertmanagers:
  - scheme: http
    timeout: 10s
    api_version: v2
    static_configs:
    - targets:
      - localhost:9093
`,
  },
};
