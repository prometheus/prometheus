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
  BoxProps,
  Collapse,
  CompoundStylesApiProps,
  ElementProps,
  factory,
  Factory,
  useProps,
} from "@mantine/core";
import { useAccordionContext } from "../Accordion.context";
import { useAccordionItemContext } from "../AccordionItem.context";
import classes from "../Accordion.module.css";
import { useEffect, useState } from "react";

export type AccordionPanelStylesNames = "panel" | "content";

export interface AccordionPanelProps
  extends BoxProps,
    CompoundStylesApiProps<AccordionPanelFactory>,
    ElementProps<"div"> {
  /** Called when the panel animation completes */
  onTransitionEnd?: () => void;
}

export type AccordionPanelFactory = Factory<{
  props: AccordionPanelProps;
  ref: HTMLDivElement;
  stylesNames: AccordionPanelStylesNames;
  compound: true;
}>;

export const AccordionPanel = factory<AccordionPanelFactory>((props, ref) => {
  const { classNames, className, style, styles, vars, children, ...others } =
    useProps("AccordionPanel", null, props);

  const { value } = useAccordionItemContext();
  const ctx = useAccordionContext();

  const isActive = ctx.isItemActive(value);

  // Prometheus-specific Accordion modification: unmount children when panel is closed.
  const [showChildren, setShowChildren] = useState(isActive);
  // Hide children from DOM 200ms after collapsing the panel
  // to give the animation time to finish.
  useEffect(() => {
    let timeout: ReturnType<typeof setTimeout>;

    if (isActive) {
      setShowChildren(true);
    } else {
      timeout = setTimeout(() => setShowChildren(false), 200);
    }

    return () => clearTimeout(timeout);
  }, [isActive]);

  return (
    <Collapse
      ref={ref}
      {...ctx.getStyles("panel", { className, classNames, style, styles })}
      {...others}
      in={isActive}
      transitionDuration={ctx.transitionDuration ?? 200}
      role="region"
      id={ctx.getRegionId(value)}
      aria-labelledby={ctx.getControlId(value)}
    >
      <div {...ctx.getStyles("content", { classNames, styles })}>
        {/* Prometheus-specific Accordion modification: unmount children when panel is closed. */}
        {showChildren && children}
      </div>
    </Collapse>
  );
});

AccordionPanel.displayName = "@mantine/core/AccordionPanel";
AccordionPanel.classes = classes;
