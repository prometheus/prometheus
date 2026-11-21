import { useMantineColorScheme, rem, ActionIcon } from "@mantine/core";
import {
  IconMoonFilled,
  IconSunFilled,
  IconBrightnessFilled,
} from "@tabler/icons-react";
import { FC } from "react";
import { useI18n } from "../i18nContext";

export const ThemeSelector: FC = () => {
  const { colorScheme, setColorScheme } = useMantineColorScheme();
  const { t } = useI18n();
  const nextThemeLabel = t(
    colorScheme === "light"
      ? "theme.switchToDark"
      : colorScheme === "dark"
        ? "theme.switchToBrowserPreferred"
        : "theme.switchToLight"
  );
  const iconProps = {
    style: { width: rem(20), height: rem(20), display: "block" },
    stroke: 1.5,
  };

  return (
    <ActionIcon
      color="gray"
      title={nextThemeLabel}
      aria-label={nextThemeLabel}
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
