import {
  ActionIcon,
  Box,
  Button,
  Group,
  InputBase,
  Loader,
  Menu,
  Modal,
  Skeleton,
  useComputedColorScheme,
} from "@mantine/core";
import {
  PromQLExtension,
  newCompleteStrategy,
} from "@prometheus-io/codemirror-promql";
import { FC, Suspense, useEffect, useRef, useState } from "react";
import CodeMirror, {
  EditorState,
  EditorView,
  Prec,
  ReactCodeMirrorRef,
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
} from "@codemirror/language";
import classes from "./ExpressionInput.module.css";
import {
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
  IconBinaryTree,
  IconCopy,
  IconDotsVertical,
  IconSearch,
  IconTerminal,
  IconTrash,
} from "@tabler/icons-react";
import { useAPIQuery } from "../../api/api";
import { notifications } from "@mantine/notifications";
import { useSettings } from "../../state/settingsSlice";
import MetricsExplorer from "./MetricsExplorer/MetricsExplorer";
import ErrorBoundary from "../../components/ErrorBoundary";
import { useAppSelector } from "../../state/hooks";
import { inputIconStyle, menuIconStyle } from "../../styles";
import { HistoryCompleteStrategy } from "./HistoryCompleteStrategy";

const promqlExtension = new PromQLExtension();


interface ExpressionInputProps {
  initialExpr: string;
  metricNames: string[];
  executeQuery: (expr: string) => void;
  treeShown: boolean;
  setShowTree: (showTree: boolean) => void;
  duplicatePanel: (expr: string) => void;
  removePanel: () => void;
}

const ExpressionInput: FC<ExpressionInputProps> = ({
  initialExpr,
  metricNames,
  executeQuery,
  duplicatePanel,
  removePanel,
  treeShown,
  setShowTree,
}) => {
  const theme = useComputedColorScheme();
  const { queryHistory } = useAppSelector((state) => state.queryPage);
  const {
    pathPrefix,
    enableAutocomplete,
    enableSyntaxHighlighting,
    enableLinter,
    enableQueryHistory,
  } = useSettings();
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
    path: "/format_query",
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
      setExpr(formatResult.data);
      notifications.show({
        title: "Expression formatted",
        message: "Expression formatted successfully!",
      });
    }
  }, [formatResult, formatError]);

  const cmRef = useRef<ReactCodeMirrorRef>(null);

  const [showMetricsExplorer, setShowMetricsExplorer] = useState(false);

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
              cache: { initialMetricList: metricNames },
            },
          }),
          enableQueryHistory ? queryHistory : []
        ),
      });
  }, [
    pathPrefix,
    metricNames,
    enableAutocomplete,
    enableLinter,
    enableQueryHistory,
    queryHistory,
  ]); // TODO: Maybe use dynamic config compartment again as in the old UI?

  return (
    <Group align="flex-start" wrap="nowrap" gap="xs">
      {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
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
                aria-label="Show query options"
              >
                <IconDotsVertical style={inputIconStyle} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Label>Query options</Menu.Label>
              <Menu.Item
                leftSection={<IconSearch style={menuIconStyle} />}
                onClick={() => setShowMetricsExplorer(true)}
              >
                Explore metrics
              </Menu.Item>
              <Menu.Item
                leftSection={<IconAlignJustified style={menuIconStyle} />}
                onClick={() => formatQuery()}
                disabled={
                  isFormatting || expr === "" || expr === formatResult?.data
                }
              >
                Format expression
              </Menu.Item>
              <Menu.Item
                leftSection={<IconBinaryTree style={menuIconStyle} />}
                onClick={() => setShowTree(!treeShown)}
              >
                {treeShown ? "Hide" : "Show"} tree view
              </Menu.Item>
              <Menu.Item
                leftSection={<IconCopy style={menuIconStyle} />}
                onClick={() => duplicatePanel(expr)}
              >
                Duplicate query
              </Menu.Item>
              <Menu.Item
                color="red"
                leftSection={<IconTrash style={menuIconStyle} />}
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
        ref={cmRef}
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
          enableSyntaxHighlighting
            ? syntaxHighlighting(
                theme === "light" ? promqlHighlighter : darkPromqlHighlighter
              )
            : [],
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

      <Button
        variant="primary"
        onClick={() => executeQuery(expr)}
        // Without this, the button can be squeezed to a width
        // that doesn't fit its text when the window is too narrow.
        style={{ flexShrink: 0 }}
      >
        Execute
      </Button>
      <Modal
        size="95%"
        opened={showMetricsExplorer}
        onClose={() => setShowMetricsExplorer(false)}
        title="Explore metrics"
      >
        <ErrorBoundary key={location.pathname} title="Error showing metrics">
          <Suspense
            fallback={
              <Box mt="lg">
                {Array.from(Array(20), (_, i) => (
                  <Skeleton key={i} height={30} mb={15} width="100%" />
                ))}
              </Box>
            }
          >
            <MetricsExplorer
              metricNames={metricNames}
              insertText={(text: string) => {
                if (cmRef.current && cmRef.current.view) {
                  const view = cmRef.current.view;
                  view.dispatch(
                    view.state.update({
                      changes: {
                        from: view.state.selection.ranges[0].from,
                        to: view.state.selection.ranges[0].to,
                        insert: text,
                      },
                    })
                  );
                }
              }}
              close={() => setShowMetricsExplorer(false)}
            />
          </Suspense>
        </ErrorBoundary>
      </Modal>
    </Group>
  );
};

export default ExpressionInput;
