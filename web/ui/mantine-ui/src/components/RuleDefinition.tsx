import {
  ActionIcon,
  Badge,
  Box,
  Card,
  Group,
  Table,
  Tooltip,
  useComputedColorScheme,
} from "@mantine/core";
import { IconClockPause, IconClockPlay, IconSearch } from "@tabler/icons-react";
import { FC } from "react";
import { formatPrometheusDuration } from "../lib/formatTime";
import codeboxClasses from "./RuleDefinition.module.css";
import { Rule } from "../api/responseTypes/rules";
import CodeMirror, { EditorView } from "@uiw/react-codemirror";
import { syntaxHighlighting } from "@codemirror/language";
import {
  baseTheme,
  darkPromqlHighlighter,
  lightTheme,
  promqlHighlighter,
} from "../codemirror/theme";
import { PromQLExtension } from "@prometheus-io/codemirror-promql";
import { LabelBadges } from "./LabelBadges";
import { useSettings } from "../state/settingsSlice";
import { actionIconStyle, badgeIconStyle } from "../styles";

const promqlExtension = new PromQLExtension();

const RuleDefinition: FC<{ rule: Rule }> = ({ rule }) => {
  const theme = useComputedColorScheme();
  const { pathPrefix } = useSettings();

  return (
    <>
      <Card p="xs" className={codeboxClasses.codebox} fz="sm" shadow="none">
        <CodeMirror
          basicSetup={false}
          value={rule.query}
          editable={false}
          extensions={[
            baseTheme,
            lightTheme,
            syntaxHighlighting(
              theme === "light" ? promqlHighlighter : darkPromqlHighlighter
            ),
            promqlExtension.asExtension(),
            EditorView.lineWrapping,
          ]}
        />

        <Tooltip label={"Query rule expression"} withArrow position="top">
          <ActionIcon
            pos="absolute"
            top={7}
            right={7}
            variant="light"
            onClick={() => {
              window.open(
                `${pathPrefix}/query?g0.expr=${encodeURIComponent(rule.query)}&g0.tab=1`,
                "_blank"
              );
            }}
            className={codeboxClasses.queryButton}
          >
            <IconSearch style={actionIconStyle} />
          </ActionIcon>
        </Tooltip>
      </Card>
      {rule.type === "alerting" && (
        <Group mt="lg" gap="xs">
          {rule.duration && (
            <Badge
              variant="light"
              styles={{ label: { textTransform: "none" } }}
              leftSection={<IconClockPause style={badgeIconStyle} />}
            >
              for: {formatPrometheusDuration(rule.duration * 1000)}
            </Badge>
          )}
          {rule.keepFiringFor && (
            <Badge
              variant="light"
              styles={{ label: { textTransform: "none" } }}
              leftSection={<IconClockPlay style={badgeIconStyle} />}
            >
              keep_firing_for: {formatPrometheusDuration(rule.keepFiringFor * 1000)}
            </Badge>
          )}
        </Group>
      )}
      {rule.labels && Object.keys(rule.labels).length > 0 && (
        <Box mt="lg">
          <LabelBadges labels={rule.labels} />
        </Box>
      )}
      {rule.type === "alerting" && Object.keys(rule.annotations).length > 0 && (
        <Table mt="lg" fz="sm">
          <Table.Tbody>
            {Object.entries(rule.annotations).map(([k, v]) => (
              <Table.Tr key={k}>
                <Table.Th c="light-dark(var(--mantine-color-gray-7), var(--mantine-color-gray-4))">
                  {k}
                </Table.Th>
                <Table.Td>{v}</Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}
    </>
  );
};

export default RuleDefinition;
