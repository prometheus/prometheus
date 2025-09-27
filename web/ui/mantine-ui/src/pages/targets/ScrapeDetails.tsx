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

type ScrapeDetailsProps = {
  target: Target;
};

const ScrapeDetails: FC<ScrapeDetailsProps> = ({ target }) => {
  const [showDetails, { toggle: toggleDetails }] = useDisclosure(false);

  return (
    <Stack gap="xs">
      <Group wrap="nowrap" align="flex-start">
        <Group gap="xs" wrap="wrap">
          <Tooltip label="Last target scrape" withArrow>
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

          <Tooltip
            label={
              <div style={{ lineHeight: 1.2 }}>
                <div>Interval: {target.scrapeInterval}</div>
                <div>Timeout: {target.scrapeTimeout}</div>
              </div>
            }
            withArrow
          >
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
          title={`${showDetails ? "Hide" : "Show"} scrape configuration details`}
        >
          {showDetails ? (
            <IconChevronUp style={actionIconStyle} />
          ) : (
            <IconChevronDown style={actionIconStyle} />
          )}
        </ActionIcon>
      </Group>

      <Collapse in={showDetails}>
        {/* Additionally remove DOM elements when not expanded (helps performance) */}
        {showDetails && (
          <Group gap="xs" wrap="wrap">
            <Tooltip label="Scrape interval" withArrow>
              <Badge
                variant="light"
                className={badgeClasses.statsBadge}
                styles={{ label: { textTransform: "none" } }}
                leftSection={<IconRepeat style={badgeIconStyle} />}
              >
                every {target.scrapeInterval}
              </Badge>
            </Tooltip>
            <Tooltip label="Scrape timeout" withArrow>
              <Badge
                variant="light"
                className={badgeClasses.statsBadge}
                styles={{ label: { textTransform: "none" } }}
                leftSection={<IconPlugConnectedX style={badgeIconStyle} />}
              >
                after {target.scrapeTimeout}
              </Badge>
            </Tooltip>
          </Group>
        )}
      </Collapse>
    </Stack>
  );
};

export default ScrapeDetails;
