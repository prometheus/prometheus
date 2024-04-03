import { Badge, BadgeVariant, Group, MantineColor } from "@mantine/core";
import { FC } from "react";
import { escapeString } from "./lib/escapeString";
import badgeClasses from "./Badge.module.css";

export interface LabelBadgesProps {
  labels: Record<string, string>;
  variant?: BadgeVariant;
  color?: MantineColor;
}

export const LabelBadges: FC<LabelBadgesProps> = ({
  labels,
  variant,
  color,
}) => (
  <Group gap="xs">
    {Object.entries(labels).map(([k, v]) => {
      return (
        <Badge
          variant={variant ? variant : "light"}
          color={color ? color : undefined}
          className={badgeClasses.labelBadge}
          styles={{
            label: {
              textTransform: "none",
            },
          }}
          key={k}
        >
          {k}="{escapeString(v)}"
        </Badge>
      );
    })}
  </Group>
);
