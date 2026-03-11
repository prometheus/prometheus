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
  ElementProps,
  factory,
  Factory,
  useProps,
} from "@mantine/core";
import { useAccordionContext } from "../Accordion.context";
import { AccordionItemProvider } from "../AccordionItem.context";
import classes from "../Accordion.module.css";

export type AccordionItemStylesNames = "item";

export interface AccordionItemProps
  extends BoxProps,
    CompoundStylesApiProps<AccordionItemFactory>,
    ElementProps<"div"> {
  /** Value that is used to manage the accordion state */
  value: string;
}

export type AccordionItemFactory = Factory<{
  props: AccordionItemProps;
  ref: HTMLDivElement;
  stylesNames: AccordionItemStylesNames;
  compound: true;
}>;

export const AccordionItem = factory<AccordionItemFactory>((props, ref) => {
  const { classNames, className, style, styles, vars, value, mod, ...others } =
    useProps("AccordionItem", null, props);
  const ctx = useAccordionContext();

  return (
    <AccordionItemProvider value={{ value }}>
      <Box
        ref={ref}
        mod={[{ active: ctx.isItemActive(value) }, mod]}
        {...ctx.getStyles("item", {
          className,
          classNames,
          styles,
          style,
          variant: ctx.variant,
        })}
        {...others}
      />
    </AccordionItemProvider>
  );
});

AccordionItem.displayName = "@mantine/core/AccordionItem";
AccordionItem.classes = classes;
