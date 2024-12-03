import { Badge, BadgeVariant, Group, MantineColor, Stack } from "@mantine/core";
import { FC } from "react";
import { escapeString } from "../lib/escapeString";
import badgeClasses from "../Badge.module.css";
import { maybeQuoteLabelName } from "../promql/utils";

export interface LabelBadgesProps {
  labels: Record<string, string>;
  variant?: BadgeVariant;
  color?: MantineColor;
  wrapper?: typeof Group | typeof Stack;
}

export const LabelBadges: FC<LabelBadgesProps> = ({
  labels,
  variant,
  color,
  wrapper: Wrapper = Group,
}) => (
  <Wrapper gap="xs">
    {Object.entries(labels).map(([k, v]) => {
      return (
        <Badge
          variant={variant ? variant : "light"}
          color={color ? color : undefined}
          className={color ? undefined : badgeClasses.labelBadge}
          styles={{
            label: {
              textTransform: "none",
            },
          }}
          key={k}
        >
          {maybeQuoteLabelName(k)}="{escapeString(v)}"
        </Badge>
      );
    })}
  </Wrapper>
);
