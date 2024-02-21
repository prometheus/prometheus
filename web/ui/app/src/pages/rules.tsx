import {
  Alert,
  Badge,
  Card,
  Group,
  Table,
  Text,
  Tooltip,
  useComputedColorScheme,
} from "@mantine/core";
// import { useQuery } from "react-query";
import {
  formatDuration,
  formatRelative,
  humanizeDuration,
  now,
} from "../lib/time-format";
import {
  IconAlertTriangle,
  IconBell,
  IconClockPause,
  IconClockPlay,
  IconDatabaseImport,
  IconHourglass,
  IconRefresh,
  IconRepeat,
} from "@tabler/icons-react";
import CodeMirror, { EditorView } from "@uiw/react-codemirror";
import { useSuspenseAPIQuery } from "../api/api";
import { RulesMap } from "../api/response-types/rules";
import { syntaxHighlighting } from "@codemirror/language";
import {
  baseTheme,
  darkPromqlHighlighter,
  lightTheme,
  promqlHighlighter,
} from "../codemirror/theme";
import { PromQLExtension } from "@prometheus-io/codemirror-promql";
import classes from "../codebox.module.css";

const healthBadgeClass = (state: string) => {
  switch (state) {
    case "ok":
      return classes.healthOk;
    case "err":
      return classes.healthErr;
    case "unknown":
      return classes.healthUnknown;
    default:
      return "orange";
  }
};

const promqlExtension = new PromQLExtension();

export default function Rules() {
  const { data } = useSuspenseAPIQuery<RulesMap>(`/rules`);
  const theme = useComputedColorScheme();

  return (
    <>
      {data.data.groups.map((g) => (
        <Card
          shadow="xs"
          withBorder
          radius="md"
          p="md"
          mb="md"
          key={g.name + ";" + g.file}
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
                  className={classes.statsBadge}
                  styles={{ label: { textTransform: "none" } }}
                  leftSection={<IconRefresh size={12} />}
                >
                  {formatRelative(g.lastEvaluation, now())}
                </Badge>
              </Tooltip>
              <Tooltip label="Duration of last group evaluation" withArrow>
                <Badge
                  variant="light"
                  className={classes.statsBadge}
                  styles={{ label: { textTransform: "none" } }}
                  leftSection={<IconHourglass size={12} />}
                >
                  {humanizeDuration(parseFloat(g.evaluationTime) * 1000)}
                </Badge>
              </Tooltip>
              <Tooltip label="Group evaluation interval" withArrow>
                <Badge
                  variant="light"
                  className={classes.statsBadge}
                  styles={{ label: { textTransform: "none" } }}
                  leftSection={<IconRepeat size={12} />}
                >
                  {humanizeDuration(parseFloat(g.interval) * 1000)}
                </Badge>
              </Tooltip>
            </Group>
          </Group>
          <Table>
            <Table.Tbody>
              {g.rules.map((r) => (
                <Table.Tr key={r.name}>
                  <Table.Td p="md" valign="top">
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
                            className={classes.statsBadge}
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
                            className={classes.statsBadge}
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
                  <Table.Td p="md">
                    <Card
                      p="xs"
                      className={classes.codebox}
                      radius="sm"
                      shadow="none"
                    >
                      <CodeMirror
                        basicSetup={false}
                        value={r.query}
                        editable={false}
                        extensions={[
                          baseTheme,
                          lightTheme,
                          syntaxHighlighting(
                            theme === "light"
                              ? promqlHighlighter
                              : darkPromqlHighlighter
                          ),
                          promqlExtension.asExtension(),
                          EditorView.lineWrapping,
                        ]}
                      />
                    </Card>

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
                    {r.type === "alerting" && (
                      <Group mt="md" gap="xs">
                        {r.duration && (
                          <Badge
                            variant="light"
                            styles={{ label: { textTransform: "none" } }}
                            leftSection={<IconClockPause size={12} />}
                          >
                            for: {formatDuration(r.duration * 1000)}
                          </Badge>
                        )}
                        {r.keepFiringFor && (
                          <Badge
                            variant="light"
                            styles={{ label: { textTransform: "none" } }}
                            leftSection={<IconClockPlay size={12} />}
                          >
                            keep_firing_for: {formatDuration(r.duration * 1000)}
                          </Badge>
                        )}
                      </Group>
                    )}
                    {r.labels && Object.keys(r.labels).length > 0 && (
                      <Group mt="md" gap="xs">
                        {Object.entries(r.labels).map(([k, v]) => (
                          <Badge
                            variant="light"
                            className={classes.labelBadge}
                            styles={{ label: { textTransform: "none" } }}
                            key={k}
                          >
                            {k}: {v}
                          </Badge>
                        ))}
                      </Group>
                    )}
                    {/* {Object.keys(r.annotations).length > 0 && (
                      <Group mt="md" gap="xs">
                        {Object.entries(r.annotations).map(([k, v]) => (
                          <Badge
                            variant="light"
                            color="orange.9"
                            styles={{ label: { textTransform: "none" } }}
                            key={k}
                          >
                            {k}: {v}
                          </Badge>
                        ))}
                      </Group>
                    )} */}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Card>
      ))}
    </>
  );
}
