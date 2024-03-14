import { Badge, Card, Group, useComputedColorScheme } from "@mantine/core";
import { IconClockPause, IconClockPlay } from "@tabler/icons-react";
import { FC } from "react";
import { formatDuration } from "./lib/formatTime";
import codeboxClasses from "./codebox.module.css";
import badgeClasses from "./Badge.module.css";
import { Rule } from "./api/responseTypes/rules";
import CodeMirror, { EditorView } from "@uiw/react-codemirror";
import { syntaxHighlighting } from "@codemirror/language";
import {
  baseTheme,
  darkPromqlHighlighter,
  lightTheme,
  promqlHighlighter,
} from "./codemirror/theme";
import { PromQLExtension } from "@prometheus-io/codemirror-promql";

const promqlExtension = new PromQLExtension();

const RuleDefinition: FC<{ rule: Rule }> = ({ rule }) => {
  const theme = useComputedColorScheme();

  return (
    <>
      <Card p="xs" className={codeboxClasses.codebox} shadow="xs">
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
      </Card>
      {rule.type === "alerting" && (
        <Group mt="md" gap="xs">
          {rule.duration && (
            <Badge
              variant="light"
              styles={{ label: { textTransform: "none" } }}
              leftSection={<IconClockPause size={12} />}
            >
              for: {formatDuration(rule.duration * 1000)}
            </Badge>
          )}
          {rule.keepFiringFor && (
            <Badge
              variant="light"
              styles={{ label: { textTransform: "none" } }}
              leftSection={<IconClockPlay size={12} />}
            >
              keep_firing_for: {formatDuration(rule.duration * 1000)}
            </Badge>
          )}
        </Group>
      )}
      {rule.labels && Object.keys(rule.labels).length > 0 && (
        <Group mt="md" gap="xs">
          {Object.entries(rule.labels).map(([k, v]) => (
            <Badge
              variant="light"
              className={badgeClasses.labelBadge}
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
    </>
  );
};

export default RuleDefinition;
