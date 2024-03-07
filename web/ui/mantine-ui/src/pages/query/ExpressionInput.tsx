import {
  ActionIcon,
  Button,
  Group,
  InputBase,
  Loader,
  Menu,
  rem,
  useComputedColorScheme,
} from "@mantine/core";
import {
  CompleteStrategy,
  PromQLExtension,
  newCompleteStrategy,
} from "@prometheus-io/codemirror-promql";
import { FC, useEffect, useState } from "react";
import CodeMirror, {
  EditorState,
  EditorView,
  Prec,
  highlightSpecialChars,
  keymap,
  placeholder,
} from "@uiw/react-codemirror";
import {
  baseTheme,
  darkPromqlHighlighter,
  darkTheme,
  lightTheme,
  promqlHighlighter,
} from "../../codemirror/theme";
import {
  bracketMatching,
  indentOnInput,
  syntaxHighlighting,
  syntaxTree,
} from "@codemirror/language";
import classes from "./ExpressionInput.module.css";
import {
  CompletionContext,
  CompletionResult,
  autocompletion,
  closeBrackets,
  closeBracketsKeymap,
  completionKeymap,
} from "@codemirror/autocomplete";
import {
  defaultKeymap,
  history,
  historyKeymap,
  insertNewlineAndIndent,
} from "@codemirror/commands";
import { highlightSelectionMatches } from "@codemirror/search";
import { lintKeymap } from "@codemirror/lint";
import {
  IconAlignJustified,
  IconDotsVertical,
  IconSearch,
  IconTerminal,
  IconTrash,
} from "@tabler/icons-react";
import { useAPIQuery } from "../../api/api";
import { notifications } from "@mantine/notifications";

const promqlExtension = new PromQLExtension();

// Autocompletion strategy that wraps the main one and enriches
// it with past query items.
export class HistoryCompleteStrategy implements CompleteStrategy {
  private complete: CompleteStrategy;
  private queryHistory: string[];
  constructor(complete: CompleteStrategy, queryHistory: string[]) {
    this.complete = complete;
    this.queryHistory = queryHistory;
  }

  promQL(
    context: CompletionContext
  ): Promise<CompletionResult | null> | CompletionResult | null {
    return Promise.resolve(this.complete.promQL(context)).then((res) => {
      const { state, pos } = context;
      const tree = syntaxTree(state).resolve(pos, -1);
      const start = res != null ? res.from : tree.from;

      if (start !== 0) {
        return res;
      }

      const historyItems: CompletionResult = {
        from: start,
        to: pos,
        options: this.queryHistory.map((q) => ({
          label: q.length < 80 ? q : q.slice(0, 76).concat("..."),
          detail: "past query",
          apply: q,
          info: q.length < 80 ? undefined : q,
        })),
        validFor: /^[a-zA-Z0-9_:]+$/,
      };

      if (res !== null) {
        historyItems.options = historyItems.options.concat(res.options);
      }
      return historyItems;
    });
  }
}

interface ExpressionInputProps {
  initialExpr: string;
  executeQuery: (expr: string) => void;
  removePanel: () => void;
}

const ExpressionInput: FC<ExpressionInputProps> = ({
  initialExpr,
  executeQuery,
  removePanel,
}) => {
  const theme = useComputedColorScheme();
  const [expr, setExpr] = useState(initialExpr);
  useEffect(() => {
    setExpr(initialExpr);
  }, [initialExpr]);

  const {
    data: formatResult,
    error: formatError,
    isFetching: isFormatting,
    refetch: formatQuery,
  } = useAPIQuery<string>({
    key: expr,
    path: "format_query",
    params: {
      query: expr,
    },
    enabled: false,
  });

  useEffect(() => {
    if (formatError) {
      notifications.show({
        color: "red",
        title: "Error formatting query",
        message: formatError.message,
      });
      return;
    }

    if (formatResult) {
      if (formatResult.status !== "success") {
        // TODO: Remove this case and handle it in useAPIQuery instead!
        return;
      }
      setExpr(formatResult.data);
      notifications.show({
        color: "green",
        title: "Expression formatted",
        message: "Expression formatted successfully!",
      });
    }
  }, [formatResult, formatError]);

  // TODO: make dynamic:
  const enableAutocomplete = true;
  const enableLinter = true;
  const pathPrefix = "";
  // const metricNames = ...
  const queryHistory = [] as string[];

  // (Re)initialize editor based on settings / setting changes.
  useEffect(() => {
    // Build the dynamic part of the config.
    promqlExtension
      .activateCompletion(enableAutocomplete)
      .activateLinter(enableLinter)
      .setComplete({
        completeStrategy: new HistoryCompleteStrategy(
          newCompleteStrategy({
            remote: {
              url: pathPrefix,
              //cache: { initialMetricList: metricNames },
            },
          }),
          queryHistory
        ),
      });
  }, []); // TODO: Make this depend on external settings changes, maybe use dynamic config compartment again.

  return (
    <Group align="flex-start" wrap="nowrap" gap="xs">
      <InputBase<any>
        leftSection={
          isFormatting ? <Loader size="xs" color="gray.5" /> : <IconTerminal />
        }
        rightSection={
          <Menu shadow="md" width={200}>
            <Menu.Target>
              <ActionIcon
                size="lg"
                variant="transparent"
                color="gray"
                aria-label="Decrease range"
              >
                <IconDotsVertical style={{ width: "1rem", height: "1rem" }} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Label>Query options</Menu.Label>
              <Menu.Item
                leftSection={
                  <IconSearch style={{ width: rem(14), height: rem(14) }} />
                }
              >
                Explore metrics
              </Menu.Item>
              <Menu.Item
                leftSection={
                  <IconAlignJustified
                    style={{ width: rem(14), height: rem(14) }}
                  />
                }
                onClick={() => formatQuery()}
                disabled={
                  isFormatting ||
                  expr === "" ||
                  (formatResult?.status === "success" &&
                    expr === formatResult.data)
                }
              >
                Format expression
              </Menu.Item>
              <Menu.Item
                color="red"
                leftSection={
                  <IconTrash style={{ width: rem(14), height: rem(14) }} />
                }
                onClick={removePanel}
              >
                Remove query
              </Menu.Item>
            </Menu.Dropdown>
          </Menu>
        }
        component={CodeMirror}
        className={classes.input}
        basicSetup={false}
        value={expr}
        onChange={setExpr}
        autoFocus
        extensions={[
          baseTheme,
          highlightSpecialChars(),
          history(),
          EditorState.allowMultipleSelections.of(true),
          indentOnInput(),
          bracketMatching(),
          closeBrackets(),
          autocompletion(),
          highlightSelectionMatches(),
          EditorView.lineWrapping,
          keymap.of([
            ...closeBracketsKeymap,
            ...defaultKeymap,
            ...historyKeymap,
            ...completionKeymap,
            ...lintKeymap,
          ]),
          placeholder("Enter expression (press Shift+Enter for newlines)"),
          syntaxHighlighting(
            theme === "light" ? promqlHighlighter : darkPromqlHighlighter
          ),
          promqlExtension.asExtension(),
          theme === "light" ? lightTheme : darkTheme,
          keymap.of([
            {
              key: "Escape",
              run: (v: EditorView): boolean => {
                v.contentDOM.blur();
                return false;
              },
            },
          ]),
          Prec.highest(
            keymap.of([
              {
                key: "Enter",
                run: (): boolean => {
                  executeQuery(expr);
                  return true;
                },
              },
              {
                key: "Shift-Enter",
                run: insertNewlineAndIndent,
              },
            ])
          ),
        ]}
        multiline
      />

      <Button variant="primary" onClick={() => executeQuery(expr)}>
        Execute
      </Button>
    </Group>
  );
};

export default ExpressionInput;
