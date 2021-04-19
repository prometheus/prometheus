import React, { FC } from 'react';

import { Story, Meta } from '@storybook/react/types-6-0';

import { withStatusIndicator, StatusIndicatorProps } from '../components/withStatusIndicator';

const dummy: FC = () => <div>dummy component</div>;

const DivWithStatusIndicator = withStatusIndicator(dummy);

export default {
  title: 'withStatusIndicator',
  component: DivWithStatusIndicator,
} as Meta;

const Template: Story<StatusIndicatorProps> = args => <DivWithStatusIndicator {...args} />;

export const NoProps = Template.bind({});
NoProps.args = {};

export const IsLoading = Template.bind({});
IsLoading.args = {
  isLoading: true,
};

export const WithError = Template.bind({});
WithError.args = {
  error: new Error('dummy error'),
};

export const WithComponentTitle = Template.bind({});
WithComponentTitle.args = {
  error: new Error('dummy error'),
  componentTitle: 'Custom Title',
};

export const WithCustomErrorMsg = Template.bind({});
WithCustomErrorMsg.args = {
  error: new Error('dummy error'),
  customErrorMsg: <div style={{ border: '1px solid #000' }}>Custom Error Title</div>,
};
