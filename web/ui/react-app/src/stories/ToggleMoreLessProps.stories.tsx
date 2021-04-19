import React from 'react';

import { Story, Meta } from '@storybook/react/types-6-0';

import { ToggleMoreLess, ToggleMoreLessProps } from '../components/ToggleMoreLess';

export default {
  title: 'ToggleMoreLess',
  component: ToggleMoreLess,
} as Meta;

const Template: Story<ToggleMoreLessProps> = args => <ToggleMoreLess {...args} />;

export const More = Template.bind({});
More.args = {
  showMore: false,
};

export const Less = Template.bind({});
Less.args = {
  showMore: true,
};
