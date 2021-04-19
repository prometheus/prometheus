import React from 'react';

import { Story, Meta } from '@storybook/react/types-6-0';

import Checkbox, { CheckboxProps } from '../components/Checkbox';

export default {
  title: 'Checkbox',
  component: Checkbox,
} as Meta;

const Template: Story<CheckboxProps> = args => <Checkbox {...args} />;

export const Checked = Template.bind({});
Checked.args = {
  checked: true,
  onChange: () => {},
};

export const Unchecked = Template.bind({});
Unchecked.args = {
  checked: false,
  onChange: () => {},
};

export const WithChildren = Template.bind({});
WithChildren.args = {
  checked: true,
  onChange: () => {},
  children: <span style={{ border: '1px solid #000' }}>Span element</span>,
};
