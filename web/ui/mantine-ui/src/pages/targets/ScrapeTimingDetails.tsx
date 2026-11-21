import { FC } from "react";
import { ActionIcon, Badge, Collapse, Group, Stack, Tooltip } from "@mantine/core";
import { useDisclosure } from "@mantine/hooks";
import {
  IconChevronDown,
  IconChevronUp,
  IconHourglass,
  IconPlugConnectedX,
  IconRefresh,
  IconRepeat,
} from "@tabler/icons-react";
import { humanizeDuration, humanizeDurationRelative, now } from "../../lib/formatTime";
import { Target } from "../../api/responseTypes/targets";
import badgeClasses from "../../Badge.module.css";
import { actionIconStyle, badgeIconStyle } from "../../styles";
import { useI18n } from "../../i18nContext";

type ScrapeTimingDetailsProps = {
  target: Target;
};

const ScrapeTimingDetails: FC<ScrapeTimingDetailsProps> = ({ target }) => {
  const { t } = useI18n();
  const [showDetails, { toggle: toggleDetails }] = useDisclosure(false);

  return (
    <Stack gap="xs">
      <Group wrap="nowrap" align="flex-start">
        <Group gap="xs" wrap="wrap">
          <Tooltip label={t("targets.lastTargetScrape")} withArrow>
            <Badge
              variant="light"
              className={badgeClasses.statsBadge}
              styles={{
                label: { textTransform: "none" },
              }}
              leftSection={<IconRefresh style={badgeIconStyle} />}
            >
              {humanizeDurationRelative(target.lastScrape, now())}
            </Badge>
          </Tooltip>

          <Tooltip label={t("targets.durationLastScrape")} withArrow>
            <Badge
              variant="light"
              className={badgeClasses.statsBadge}
              styles={{
                label: { textTransform: "none" },
              }}
              leftSection={<IconHourglass style={badgeIconStyle} />}
            >
              {humanizeDuration(target.lastScrapeDuration * 1000)}
            </Badge>
          </Tooltip>
        </Group>

        <ActionIcon
          size="xs"
          color="gray"
          variant="light"
          onClick={toggleDetails}
          title={t("targets.additionalTiming", {
            action: t(showDetails ? "targets.hide" : "targets.show"),
          })}
        >
          {showDetails ? (
            <IconChevronUp style={actionIconStyle} />
          ) : (
            <IconChevronDown style={actionIconStyle} />
          )}
        </ActionIcon>
      </Group>

      <Collapse expanded={showDetails}>
        {/* Additionally remove DOM elements when not expanded (helps performance) */}
        {showDetails && (
          <Group gap="xs" wrap="wrap">
            <Tooltip label={t("targets.scrapeInterval")} withArrow>
              <Badge
                variant="light"
                className={badgeClasses.statsBadge}
                styles={{ label: { textTransform: "none" } }}
                leftSection={<IconRepeat style={badgeIconStyle} />}
              >
                {t("targets.every", { duration: target.scrapeInterval })}
              </Badge>
            </Tooltip>
            <Tooltip label={t("targets.scrapeTimeout")} withArrow>
              <Badge
                variant="light"
                className={badgeClasses.statsBadge}
                styles={{ label: { textTransform: "none" } }}
                leftSection={<IconPlugConnectedX style={badgeIconStyle} />}
              >
                {t("targets.after", { duration: target.scrapeTimeout })}
              </Badge>
            </Tooltip>
          </Group>
        )}
      </Collapse>
    </Stack>
  );
};

export default ScrapeTimingDetails;
