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

import {
  Box,
  BoxProps,
  CompoundStylesApiProps,
  createScopedKeydownHandler,
  ElementProps,
  factory,
  Factory,
  UnstyledButton,
  useProps,
} from "@mantine/core";
import { useAccordionContext } from "../Accordion.context";
import { useAccordionItemContext } from "../AccordionItem.context";
import classes from "../Accordion.module.css";

export type AccordionControlStylesNames =
  | "control"
  | "chevron"
  | "label"
  | "itemTitle"
  | "icon";

export interface AccordionControlProps
  extends BoxProps,
    CompoundStylesApiProps<AccordionControlFactory>,
    ElementProps<"button"> {
  /** Sets `disabled` attribute, prevents interactions */
  disabled?: boolean;

  /** Custom chevron icon */
  chevron?: React.ReactNode;

  /** Control label */
  children?: React.ReactNode;

  /** Icon displayed next to the label */
  icon?: React.ReactNode;
}

export type AccordionControlFactory = Factory<{
  props: AccordionControlProps;
  ref: HTMLButtonElement;
  stylesNames: AccordionControlStylesNames;
  compound: true;
}>;

export const AccordionControl = factory<AccordionControlFactory>(
  (props, ref) => {
    const {
      classNames,
      className,
      style,
      styles,
      vars,
      chevron,
      icon,
      onClick,
      onKeyDown,
      children,
      disabled,
      mod,
      ...others
    } = useProps("AccordionControl", null, props);

    const { value } = useAccordionItemContext();
    const ctx = useAccordionContext();
    const isActive = ctx.isItemActive(value);
    const shouldWrapWithHeading = typeof ctx.order === "number";
    const Heading = `h${ctx.order!}` as const;

    const content = (
      <UnstyledButton<"button">
        {...others}
        {...ctx.getStyles("control", {
          className,
          classNames,
          style,
          styles,
          variant: ctx.variant,
        })}
        unstyled={ctx.unstyled}
        mod={[
          "accordion-control",
          {
            active: isActive,
            "chevron-position": ctx.chevronPosition,
            disabled,
          },
          mod,
        ]}
        ref={ref}
        onClick={(event) => {
          onClick?.(event);
          ctx.onChange(value);
        }}
        type="button"
        disabled={disabled}
        aria-expanded={isActive}
        aria-controls={ctx.getRegionId(value)}
        id={ctx.getControlId(value)}
        onKeyDown={createScopedKeydownHandler({
          siblingSelector: "[data-accordion-control]",
          parentSelector: "[data-accordion]",
          activateOnFocus: false,
          loop: ctx.loop,
          orientation: "vertical",
          onKeyDown,
        })}
      >
        <Box
          component="span"
          mod={{
            rotate: !ctx.disableChevronRotation && isActive,
            position: ctx.chevronPosition,
          }}
          {...ctx.getStyles("chevron", { classNames, styles })}
        >
          {chevron || ctx.chevron}
        </Box>
        <span {...ctx.getStyles("label", { classNames, styles })}>
          {children}
        </span>
        {icon && (
          <Box
            component="span"
            mod={{ "chevron-position": ctx.chevronPosition }}
            {...ctx.getStyles("icon", { classNames, styles })}
          >
            {icon}
          </Box>
        )}
      </UnstyledButton>
    );

    return shouldWrapWithHeading ? (
      <Heading {...ctx.getStyles("itemTitle", { classNames, styles })}>
        {content}
      </Heading>
    ) : (
      content
    );
  }
);

AccordionControl.displayName = "@mantine/core/AccordionControl";
AccordionControl.classes = classes;
