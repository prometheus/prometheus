import {
  Accordion,
  Alert,
  Badge,
  Card,
  Group,
  Pagination,
  rem,
  Stack,
  Text,
  Tooltip,
} from "@mantine/core";
// import { useQuery } from "react-query";
import {
  humanizeDurationRelative,
  humanizeDuration,
  now,
} from "../lib/formatTime";
import {
  IconAlertTriangle,
  IconBell,
  IconHourglass,
  IconInfoCircle,
  IconRefresh,
  IconRepeat,
  IconTimeline,
} from "@tabler/icons-react";
import { useSuspenseAPIQuery } from "../api/api";
import { RulesResult } from "../api/responseTypes/rules";
import badgeClasses from "../Badge.module.css";
import RuleDefinition from "../components/RuleDefinition";
import { badgeIconStyle } from "../styles";
import { NumberParam, useQueryParam, withDefault } from "use-query-params";
import { useSettings } from "../state/settingsSlice";
import { useEffect } from "react";
import CustomInfiniteScroll from "../components/CustomInfiniteScroll";

const healthBadgeClass = (state: string) => {
  switch (state) {
    case "ok":
      return badgeClasses.healthOk;
    case "err":
      return badgeClasses.healthErr;
    case "unknown":
      return badgeClasses.healthUnknown;
    default:
      throw new Error("Unknown rule health state");
  }
};

export default function RulesPage() {
  const { data } = useSuspenseAPIQuery<RulesResult>({ path: `/rules` });
  const { ruleGroupsPerPage } = useSettings();

  const [activePage, setActivePage] = useQueryParam(
    "page",
    withDefault(NumberParam, 1)
  );

  // If we were e.g. on page 10 and the number of total pages decreases to 5 (due
  // changing the max number of items per page), go to the largest possible page.
  const totalPageCount = Math.ceil(data.data.groups.length / ruleGroupsPerPage);
  const effectiveActivePage = Math.max(1, Math.min(activePage, totalPageCount));

  useEffect(() => {
    if (effectiveActivePage !== activePage) {
      setActivePage(effectiveActivePage);
    }
  }, [effectiveActivePage, activePage, setActivePage]);

  return (
    <Stack mt="xs">
      {data.data.groups.length === 0 && (
        <Alert title="No rule groups" icon={<IconInfoCircle />}>
          No rule groups configured.
        </Alert>
      )}
      <Pagination
        total={totalPageCount}
        value={effectiveActivePage}
        onChange={setActivePage}
        hideWithOnePage
      />
      {data.data.groups
        .slice(
          (effectiveActivePage - 1) * ruleGroupsPerPage,
          effectiveActivePage * ruleGroupsPerPage
        )
        .map((g, i) => (
          <Card
            shadow="xs"
            withBorder
            p="md"
            key={i} // TODO: Find a stable and definitely unique key.
          >
            <Group mb="sm" justify="space-between">
              <Group align="baseline">
                <Text fz="xl" fw={600} c="var(--mantine-primary-color-filled)">
                  {g.name}
                </Text>
                <Text fz="sm" c="gray.6">
                  {g.file}
                </Text>
              </Group>
              <Group>
                <Tooltip label="Last group evaluation" withArrow>
                  <Badge
                    variant="light"
                    className={badgeClasses.statsBadge}
                    styles={{ label: { textTransform: "none" } }}
                    leftSection={<IconRefresh style={badgeIconStyle} />}
                  >
                    last run {humanizeDurationRelative(g.lastEvaluation, now())}
                  </Badge>
                </Tooltip>
                <Tooltip label="Duration of last group evaluation" withArrow>
                  <Badge
                    variant="light"
                    className={badgeClasses.statsBadge}
                    styles={{ label: { textTransform: "none" } }}
                    leftSection={<IconHourglass style={badgeIconStyle} />}
                  >
                    took {humanizeDuration(parseFloat(g.evaluationTime) * 1000)}
                  </Badge>
                </Tooltip>
                <Tooltip label="Group evaluation interval" withArrow>
                  <Badge
                    variant="transparent"
                    className={badgeClasses.statsBadge}
                    styles={{ label: { textTransform: "none" } }}
                    leftSection={<IconRepeat style={badgeIconStyle} />}
                  >
                    every {humanizeDuration(parseFloat(g.interval) * 1000)}{" "}
                  </Badge>
                </Tooltip>
              </Group>
            </Group>
            {g.rules.length === 0 && (
              <Alert title="No rules" icon={<IconInfoCircle />}>
                No rules in rule group.
              </Alert>
            )}
            <CustomInfiniteScroll
              allItems={g.rules}
              child={({ items }) => (
                <Accordion multiple variant="separated">
                  {items.map((r, j) => (
                    <Accordion.Item
                      mt={rem(5)}
                      styles={{
                        item: {
                          // TODO: This transparency hack is an OK workaround to make the collapsed items
                          // have a different background color than their surrounding group card in dark mode,
                          // but it would be better to use CSS to override the light/dark colors for
                          // collapsed/expanded accordion items.
                          backgroundColor: "#c0c0c015",
                        },
                      }}
                      key={j}
                      value={j.toString()}
                      style={{
                        borderLeft:
                          r.health === "err"
                            ? "5px solid var(--mantine-color-red-4)"
                            : r.health === "unknown"
                              ? "5px solid var(--mantine-color-gray-5)"
                              : "5px solid var(--mantine-color-green-4)",
                      }}
                    >
                      <Accordion.Control
                        styles={{ label: { paddingBlock: rem(10) } }}
                      >
                        <Group justify="space-between" mr="lg">
                          <Group gap="xs" wrap="nowrap">
                            {r.type === "alerting" ? (
                              <Tooltip label="Alerting rule" withArrow>
                                <IconBell
                                  style={{ width: rem(15), height: rem(15) }}
                                />
                              </Tooltip>
                            ) : (
                              <Tooltip label="Recording rule" withArrow>
                                <IconTimeline
                                  style={{ width: rem(15), height: rem(15) }}
                                />
                              </Tooltip>
                            )}
                            <Text>{r.name}</Text>
                          </Group>
                          <Group gap="xs">
                            <Group gap="xs" wrap="wrap">
                              <Tooltip label="Last rule evaluation" withArrow>
                                <Badge
                                  variant="light"
                                  className={badgeClasses.statsBadge}
                                  styles={{ label: { textTransform: "none" } }}
                                  leftSection={
                                    <IconRefresh style={badgeIconStyle} />
                                  }
                                >
                                  {humanizeDurationRelative(
                                    r.lastEvaluation,
                                    now()
                                  )}
                                </Badge>
                              </Tooltip>

                              <Tooltip
                                label="Duration of last rule evaluation"
                                withArrow
                              >
                                <Badge
                                  variant="light"
                                  className={badgeClasses.statsBadge}
                                  styles={{ label: { textTransform: "none" } }}
                                  leftSection={
                                    <IconHourglass style={badgeIconStyle} />
                                  }
                                >
                                  {humanizeDuration(
                                    parseFloat(r.evaluationTime) * 1000
                                  )}
                                </Badge>
                              </Tooltip>
                            </Group>
                            <Badge className={healthBadgeClass(r.health)}>
                              {r.health}
                            </Badge>
                          </Group>
                        </Group>
                      </Accordion.Control>
                      <Accordion.Panel>
                        <RuleDefinition rule={r} />
                        {r.lastError && (
                          <Alert
                            color="red"
                            mt="sm"
                            title="Rule failed to evaluate"
                            icon={<IconAlertTriangle />}
                          >
                            <strong>Error:</strong> {r.lastError}
                          </Alert>
                        )}
                      </Accordion.Panel>
                    </Accordion.Item>
                  ))}
                </Accordion>
              )}
            />
          </Card>
        ))}
    </Stack>
  );
}
