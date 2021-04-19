import React from 'react';

import { Story, Meta } from '@storybook/react/types-6-0';

import AlertsContent, { AlertsProps } from '../pages/alerts/AlertContents';
import alertRules from './__fixtures__/alerts';

export default {
  title: 'AlertsContent',
  component: AlertsContent,
} as Meta;

const Template: Story<AlertsProps> = args => <AlertsContent {...args} />;

export const NoAlerts = Template.bind({});
NoAlerts.args = {
  groups: [],
  statsCount: {
    inactive: 0,
    pending: 0,
    firing: 0,
  },
};

export const LongAlerts = Template.bind({});
LongAlerts.args = {
  groups: alertRules,
  statsCount: {
    inactive: 152,
    pending: 20,
    firing: 3333,
  },
};

export const Expanded = Template.bind({});
Expanded.args = {
  groups: alertRules,
  statsCount: {
    inactive: 152,
    pending: 20,
    firing: 3333,
  },
  defaultIsExpanded: true,
};
