import {
  Alert,
  Badge,
  Card,
  Group,
  Stack,
  Table,
  Text,
  Tooltip,
} from "@mantine/core";
// import { useQuery } from "react-query";
import { formatRelative, humanizeDuration, now } from "../lib/formatTime";
import {
  IconAlertTriangle,
  IconBell,
  IconDatabaseImport,
  IconHourglass,
  IconRefresh,
  IconRepeat,
} from "@tabler/icons-react";
import { useSuspenseAPIQuery } from "../api/api";
import { RulesMap } from "../api/responseTypes/rules";
import badgeClasses from "../Badge.module.css";
import RuleDefinition from "../RuleDefinition";

const healthBadgeClass = (state: string) => {
  switch (state) {
    case "ok":
      return badgeClasses.healthOk;
    case "err":
      return badgeClasses.healthErr;
    case "unknown":
      return badgeClasses.healthUnknown;
    default:
      return "orange";
  }
};

export default function RulesPage() {
  const { data } = useSuspenseAPIQuery<RulesMap>({ path: `/rules` });

  return (
    <Stack mt="xs">
      {data.data.groups.map((g, i) => (
        <Card
          shadow="xs"
          withBorder
          p="md"
          mb="md"
          key={i} // TODO: Find a stable and definitely unique key.
        >
          <Group mb="md" mt="xs" ml="xs" justify="space-between">
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
                  leftSection={<IconRefresh size={12} />}
                >
                  last run {formatRelative(g.lastEvaluation, now())}
                </Badge>
              </Tooltip>
              <Tooltip label="Duration of last group evaluation" withArrow>
                <Badge
                  variant="light"
                  className={badgeClasses.statsBadge}
                  styles={{ label: { textTransform: "none" } }}
                  leftSection={<IconHourglass size={12} />}
                >
                  took {humanizeDuration(parseFloat(g.evaluationTime) * 1000)}
                </Badge>
              </Tooltip>
              <Tooltip label="Group evaluation interval" withArrow>
                <Badge
                  variant="transparent"
                  className={badgeClasses.statsBadge}
                  styles={{ label: { textTransform: "none" } }}
                  leftSection={<IconRepeat size={12} />}
                >
                  every {humanizeDuration(parseFloat(g.interval) * 1000)}{" "}
                </Badge>
              </Tooltip>
            </Group>
          </Group>
          <Table>
            <Table.Tbody>
              {g.rules.map((r) => (
                // TODO: Find a stable and definitely unique key.
                <Table.Tr key={r.name}>
                  <Table.Td p="md" py="xl" valign="top">
                    <Group gap="xs" wrap="nowrap">
                      {r.type === "alerting" ? (
                        <IconBell size={14} />
                      ) : (
                        <IconDatabaseImport size={14} />
                      )}
                      <Text fz="sm" fw={600}>
                        {r.name}
                      </Text>
                    </Group>
                    <Group mt="md" gap="xs">
                      <Badge className={healthBadgeClass(r.health)}>
                        {r.health}
                      </Badge>

                      <Group gap="xs" wrap="wrap">
                        <Tooltip label="Last rule evaluation" withArrow>
                          <Badge
                            variant="light"
                            className={badgeClasses.statsBadge}
                            styles={{ label: { textTransform: "none" } }}
                            leftSection={<IconRefresh size={12} />}
                          >
                            {formatRelative(r.lastEvaluation, now())}
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
                            leftSection={<IconHourglass size={12} />}
                          >
                            {humanizeDuration(
                              parseFloat(r.evaluationTime) * 1000
                            )}
                          </Badge>
                        </Tooltip>
                      </Group>
                    </Group>
                  </Table.Td>
                  <Table.Td p="md" py="xl">
                    <RuleDefinition rule={r} />
                    {r.lastError && (
                      <Alert
                        color="red"
                        mt="sm"
                        title="Rule failed to evaluate"
                        icon={<IconAlertTriangle size={14} />}
                      >
                        <strong>Error:</strong> {r.lastError}
                      </Alert>
                    )}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Card>
      ))}
    </Stack>
  );
}
