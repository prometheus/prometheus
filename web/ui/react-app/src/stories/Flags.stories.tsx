import React from 'react';

import { Story, Meta } from '@storybook/react/types-6-0';

import { FlagsContent, FlagsProps } from '../pages/flags/Flags';
import { flags } from './__fixtures__/flags';

export default {
  title: 'FlagsContent',
  component: FlagsContent,
} as Meta;

const Template: Story<FlagsProps> = args => <FlagsContent {...args} />;

export const EmptyData = Template.bind({});
EmptyData.args = {
  data: {},
};

export const ExampleFlags = Template.bind({});
ExampleFlags.args = {
  data: flags,
};
