import {
  useMantineColorScheme,
  SegmentedControl,
  rem,
  MantineColorScheme,
  Tooltip,
} from "@mantine/core";
import {
  IconMoonFilled,
  IconSunFilled,
  IconUserFilled,
} from "@tabler/icons-react";
import { FC } from "react";

export const ThemeSelector: FC = () => {
  const { colorScheme, setColorScheme } = useMantineColorScheme();
  const iconProps = {
    style: { width: rem(20), height: rem(20), display: "block" },
    stroke: 1.5,
  };

  return (
    <SegmentedControl
      color="gray.7"
      size="xs"
      // styles={{ root: { backgroundColor: "var(--mantine-color-gray-7)" } }}
      styles={{
        root: {
          padding: 3,
          backgroundColor: "var(--mantine-color-gray-6)",
        },
      }}
      withItemsBorders={false}
      value={colorScheme}
      onChange={(v) => setColorScheme(v as MantineColorScheme)}
      data={[
        {
          value: "light",
          label: (
            <Tooltip label="Use light theme" offset={15}>
              <IconSunFilled {...iconProps} />
            </Tooltip>
          ),
        },
        {
          value: "dark",
          label: (
            <Tooltip label="Use dark theme" offset={15}>
              <IconMoonFilled {...iconProps} />
            </Tooltip>
          ),
        },
        {
          value: "auto",
          label: (
            <Tooltip label="Use browser-preferred theme" offset={15}>
              <IconUserFilled {...iconProps} />
            </Tooltip>
          ),
        },
      ]}
    />
  );
};
