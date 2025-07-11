import { Group, Stack } from "@mantine/core";
import { FC } from "react";
import { escapeString } from "../lib/escapeString";
import badgeClasses from "../Badge.module.css";
import { maybeQuoteLabelName } from "../promql/utils";

export interface LabelBadgesProps {
  labels: Record<string, string>;
  wrapper?: typeof Group | typeof Stack;
  style?: React.CSSProperties;
}

export const LabelBadges: FC<LabelBadgesProps> = ({
  labels,
  wrapper: Wrapper = Group,
  style,
}) => (
  <Wrapper gap="xs">
    {Object.entries(labels).map(([k, v]) => (
      // We build our own Mantine-style badges here for performance
      // reasons (see comment in Badge.module.css).
      <span key={k} className={badgeClasses.labelBadge} style={style}>
        {maybeQuoteLabelName(k)}="{escapeString(v)}"
      </span>
    ))}
  </Wrapper>
);
