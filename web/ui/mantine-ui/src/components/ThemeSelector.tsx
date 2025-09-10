import { useMantineColorScheme, rem, ActionIcon } from "@mantine/core";
import {
  IconMoonFilled,
  IconSunFilled,
  IconBrightnessFilled,
} from "@tabler/icons-react";
import { FC } from "react";

export const ThemeSelector: FC = () => {
  const { colorScheme, setColorScheme } = useMantineColorScheme();
  const iconProps = {
    style: { width: rem(20), height: rem(20), display: "block" },
    stroke: 1.5,
  };

  return (
    <ActionIcon
      color="gray"
      title={`Switch to ${colorScheme === "light" ? "dark" : colorScheme === "dark" ? "browser-preferred" : "light"} theme`}
      aria-label={`Switch to ${colorScheme === "light" ? "dark" : colorScheme === "dark" ? "browser-preferred" : "light"} theme`}
      size={32}
      onClick={() =>
        setColorScheme(
          colorScheme === "light"
            ? "dark"
            : colorScheme === "dark"
              ? "auto"
              : "light"
        )
      }
    >
      {colorScheme === "light" ? (
        <IconSunFilled {...iconProps} />
      ) : colorScheme === "dark" ? (
        <IconMoonFilled {...iconProps} />
      ) : (
        <IconBrightnessFilled {...iconProps} />
      )}
    </ActionIcon>
  );
};
