/*
 * Some parts of this file are derived from Mantine UI (https://github.com/mantinedev/mantine)
 * which is distributed under the MIT license:
 *
 * MIT License
 *
 * Copyright (c) 2021 Vitaly Rtishchev
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 *
 * Modifications to this file are licensed under the Apache License, Version 2.0.
 */

import { useId, useUncontrolled } from "@mantine/hooks";
import {
  Box,
  BoxProps,
  createVarsResolver,
  ElementProps,
  ExtendComponent,
  Factory,
  getRadius,
  getSafeId,
  getWithProps,
  MantineRadius,
  MantineThemeComponent,
  rem,
  StylesApiProps,
  useProps,
  useStyles,
} from "@mantine/core";
import { AccordionProvider } from "./Accordion.context";
import {
  AccordionChevronPosition,
  AccordionHeadingOrder,
  AccordionValue,
} from "./Accordion.types";
import { AccordionChevron } from "./AccordionChevron";
import { AccordionControl } from "./AccordionControl/AccordionControl";
import { AccordionItem } from "./AccordionItem/AccordionItem";
import { AccordionPanel } from "./AccordionPanel/AccordionPanel";
import classes from "./Accordion.module.css";

export type AccordionStylesNames =
  | "root"
  | "content"
  | "item"
  | "panel"
  | "icon"
  | "chevron"
  | "label"
  | "itemTitle"
  | "control";

export type AccordionVariant = "default" | "contained" | "filled" | "separated";
export type AccordionCssVariables = {
  root:
    | "--accordion-transition-duration"
    | "--accordion-chevron-size"
    | "--accordion-radius";
};

export interface AccordionProps<Multiple extends boolean = false>
  extends BoxProps,
    StylesApiProps<AccordionFactory>,
    ElementProps<"div", "value" | "defaultValue" | "onChange"> {
  /** If set, multiple items can be opened at the same time */
  multiple?: Multiple;

  /** Controlled component value */
  value?: AccordionValue<Multiple>;

  /** Uncontrolled component default value */
  defaultValue?: AccordionValue<Multiple>;

  /** Called when value changes, payload type depends on `multiple` prop */
  onChange?: (value: AccordionValue<Multiple>) => void;

  /** If set, arrow keys loop though items (first to last and last to first) @default `true` */
  loop?: boolean;

  /** Transition duration in ms @default `200` */
  transitionDuration?: number;

  /** If set, chevron rotation is disabled */
  disableChevronRotation?: boolean;

  /** Position of the chevron relative to the item label @default `right` */
  chevronPosition?: AccordionChevronPosition;

  /** Size of the chevron icon container @default `auto` */
  chevronSize?: number | string;

  /** Size of the default chevron icon. Ignored when `chevron` prop is set. @default `16` */
  chevronIconSize?: number | string;

  /** Heading order, has no effect on visuals */
  order?: AccordionHeadingOrder;

  /** Custom chevron icon */
  chevron?: React.ReactNode;

  /** Key of `theme.radius` or any valid CSS value to set border-radius. Numbers are converted to rem. @default `theme.defaultRadius` */
  radius?: MantineRadius;
}

export type AccordionFactory = Factory<{
  props: AccordionProps;
  ref: HTMLDivElement;
  stylesNames: AccordionStylesNames;
  vars: AccordionCssVariables;
  variant: AccordionVariant;
}>;

const defaultProps = {
  multiple: false,
  disableChevronRotation: false,
  chevronPosition: "right",
  variant: "default",
  chevronSize: "auto",
  chevronIconSize: 16,
} satisfies Partial<AccordionProps>;

const varsResolver = createVarsResolver<AccordionFactory>(
  (_, { transitionDuration, chevronSize, radius }) => ({
    root: {
      "--accordion-transition-duration":
        transitionDuration === undefined
          ? undefined
          : `${transitionDuration}ms`,
      "--accordion-chevron-size":
        chevronSize === undefined ? undefined : rem(chevronSize),
      "--accordion-radius":
        radius === undefined ? undefined : getRadius(radius),
    },
  })
);

export function Accordion<Multiple extends boolean = false>(
  _props: AccordionProps<Multiple>
) {
  const props = useProps(
    "Accordion",
    defaultProps as AccordionProps<Multiple>,
    _props
  );
  const {
    classNames,
    className,
    style,
    styles,
    unstyled,
    vars,
    children,
    multiple,
    value,
    defaultValue,
    onChange,
    id,
    loop,
    transitionDuration,
    disableChevronRotation,
    chevronPosition,
    chevronSize,
    order,
    chevron,
    variant,
    radius,
    chevronIconSize,
    attributes,
    ...others
  } = props;

  const uid = useId(id);
  const [_value, handleChange] = useUncontrolled({
    value,
    defaultValue,
    finalValue: multiple ? ([] as any) : null,
    onChange,
  });

  const isItemActive = (itemValue: string) =>
    Array.isArray(_value) ? _value.includes(itemValue) : itemValue === _value;

  const handleItemChange = (itemValue: string) => {
    const nextValue: AccordionValue<Multiple> = Array.isArray(_value)
      ? _value.includes(itemValue)
        ? _value.filter((selectedValue) => selectedValue !== itemValue)
        : [..._value, itemValue]
      : itemValue === _value
        ? null
        : (itemValue as any);

    handleChange(nextValue);
  };

  const getStyles = useStyles<AccordionFactory>({
    name: "Accordion",
    classes,
    props: props as AccordionProps,
    className,
    style,
    classNames,
    styles,
    unstyled,
    attributes,
    vars,
    varsResolver,
  });

  return (
    <AccordionProvider
      value={{
        isItemActive,
        onChange: handleItemChange,
        getControlId: getSafeId(
          `${uid}-control`,
          "Accordion.Item component was rendered with invalid value or without value"
        ),
        getRegionId: getSafeId(
          `${uid}-panel`,
          "Accordion.Item component was rendered with invalid value or without value"
        ),
        chevron:
          chevron === null
            ? null
            : chevron || <AccordionChevron size={chevronIconSize} />,
        transitionDuration,
        disableChevronRotation,
        chevronPosition,
        order,
        loop,
        getStyles,
        variant,
        unstyled,
      }}
    >
      <Box
        {...getStyles("root")}
        id={uid}
        {...others}
        variant={variant}
        data-accordion
      >
        {children}
      </Box>
    </AccordionProvider>
  );
}

const extendAccordion = (
  c: ExtendComponent<AccordionFactory>
): MantineThemeComponent => c;

Accordion.extend = extendAccordion;
Accordion.withProps = getWithProps<AccordionProps, AccordionProps>(
  Accordion as any
);
Accordion.classes = classes;
Accordion.displayName = "@mantine/core/Accordion";
Accordion.Item = AccordionItem;
Accordion.Panel = AccordionPanel;
Accordion.Control = AccordionControl;
Accordion.Chevron = AccordionChevron;
