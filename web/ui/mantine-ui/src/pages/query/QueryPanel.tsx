import {
  Group,
  Tabs,
  Center,
  Space,
  Box,
  SegmentedControl,
  Stack,
  Select,
  TextInput,
} from "@mantine/core";
import {
  IconChartAreaFilled,
  IconChartGridDots,
  IconChartLine,
  IconGraph,
  IconTable,
} from "@tabler/icons-react";
import { FC, useState } from "react";
import { useAppDispatch, useAppSelector } from "../../state/hooks";
import {
  GraphDisplayMode,
  GraphResolution,
  getEffectiveResolution,
  removePanel,
  setExpr,
  setVisualizer,
} from "../../state/queryPageSlice";
import DataTable from "./DataTable";
import TimeInput from "./TimeInput";
import RangeInput from "./RangeInput";
import ExpressionInput from "./ExpressionInput";
import Graph from "./Graph";
import {
  formatPrometheusDuration,
  parsePrometheusDuration,
} from "../../lib/formatTime";

export interface PanelProps {
  idx: number;
  metricNames: string[];
}

// TODO: This is duplicated everywhere, unify it.
const iconStyle = { width: "0.9rem", height: "0.9rem" };

const QueryPanel: FC<PanelProps> = ({ idx, metricNames }) => {
  // Used to indicate to the selected display component that it should retrigger
  // the query, even if the expression has not changed (e.g. when the user presses
  // the "Execute" button or hits <Enter> again).
  const [retriggerIdx, setRetriggerIdx] = useState<number>(0);

  const panel = useAppSelector((state) => state.queryPage.panels[idx]);
  const resolution = panel.visualizer.resolution;
  const dispatch = useAppDispatch();

  const [customResolutionInput, setCustomResolutionInput] = useState<string>(
    formatPrometheusDuration(
      getEffectiveResolution(resolution, panel.visualizer.range)
    )
  );

  const setResolution = (res: GraphResolution) => {
    dispatch(
      setVisualizer({
        idx,
        visualizer: {
          ...panel.visualizer,
          resolution: res,
        },
      })
    );
  };

  const onChangeCustomResolutionInput = (resText: string): void => {
    const newResolution = parsePrometheusDuration(resText);
    if (newResolution === null) {
      setCustomResolutionInput(
        formatPrometheusDuration(
          getEffectiveResolution(resolution, panel.visualizer.range)
        )
      );
    } else {
      setResolution({ type: "custom", value: newResolution });
    }
  };

  return (
    <Stack gap="lg">
      <ExpressionInput
        initialExpr={panel.expr}
        metricNames={metricNames}
        executeQuery={(expr: string) => {
          setRetriggerIdx((idx) => idx + 1);
          dispatch(setExpr({ idx, expr }));
        }}
        removePanel={() => {
          dispatch(removePanel(idx));
        }}
      />
      <Tabs defaultValue="table" keepMounted={false}>
        <Tabs.List>
          <Tabs.Tab value="table" leftSection={<IconTable style={iconStyle} />}>
            Table
          </Tabs.Tab>
          <Tabs.Tab value="graph" leftSection={<IconGraph style={iconStyle} />}>
            Graph
          </Tabs.Tab>
        </Tabs.List>
        <Tabs.Panel pt="sm" value="table">
          <Stack gap="lg" mt="sm">
            <TimeInput
              time={panel.visualizer.endTime}
              range={panel.visualizer.range}
              description="Evaluation time"
              onChangeTime={(time) =>
                dispatch(
                  setVisualizer({
                    idx,
                    visualizer: { ...panel.visualizer, endTime: time },
                  })
                )
              }
            />
            <DataTable
              expr={panel.expr}
              evalTime={panel.visualizer.endTime}
              retriggerIdx={retriggerIdx}
            />
          </Stack>
        </Tabs.Panel>
        <Tabs.Panel
          pt="sm"
          value="graph"
          // style={{ border: "1px solid lightgrey", borderTop: "none" }}
        >
          <Group mt="xs" justify="space-between">
            <Group>
              <RangeInput
                range={panel.visualizer.range}
                onChangeRange={(range) =>
                  dispatch(
                    setVisualizer({
                      idx,
                      visualizer: { ...panel.visualizer, range },
                    })
                  )
                }
              />
              <TimeInput
                time={panel.visualizer.endTime}
                range={panel.visualizer.range}
                description="End time"
                onChangeTime={(time) =>
                  dispatch(
                    setVisualizer({
                      idx,
                      visualizer: { ...panel.visualizer, endTime: time },
                    })
                  )
                }
              />

              <Select
                title="Resolution"
                placeholder="Resolution"
                maxDropdownHeight={500}
                data={[
                  {
                    group: "Automatic resolution",
                    items: [
                      { label: "Low res.", value: "low" },
                      { label: "Medium res.", value: "medium" },
                      { label: "High res.", value: "high" },
                    ],
                  },
                  {
                    group: "Fixed resolution",
                    items: [
                      { label: "10s", value: "10000" },
                      { label: "30s", value: "30000" },
                      { label: "1m", value: "60000" },
                      { label: "5m", value: "300000" },
                      { label: "15m", value: "900000" },
                      { label: "1h", value: "3600000" },
                    ],
                  },
                  {
                    group: "Custom resolution",
                    items: [{ label: "Enter value...", value: "custom" }],
                  },
                ]}
                w={160}
                value={
                  resolution.type === "auto"
                    ? resolution.density
                    : resolution.type === "fixed"
                      ? resolution.value.toString()
                      : "custom"
                }
                onChange={(_value, option) => {
                  if (["low", "medium", "high"].includes(option.value)) {
                    setResolution({
                      type: "auto",
                      density: option.value as "low" | "medium" | "high",
                    });
                    return;
                  }

                  if (option.value === "custom") {
                    // Start the custom resolution at the current effective resolution.
                    const effectiveResolution = getEffectiveResolution(
                      resolution,
                      panel.visualizer.range
                    );
                    setResolution({
                      type: "custom",
                      value: effectiveResolution,
                    });
                    setCustomResolutionInput(
                      formatPrometheusDuration(effectiveResolution)
                    );
                    return;
                  }

                  const value = parseInt(option.value);
                  if (!isNaN(value)) {
                    setResolution({
                      type: "fixed",
                      value,
                    });
                  } else {
                    throw new Error("Invalid resolution value");
                  }
                }}
              />

              {resolution.type === "custom" && (
                <TextInput
                  placeholder="Resolution"
                  value={customResolutionInput}
                  onChange={(event) =>
                    setCustomResolutionInput(event.currentTarget.value)
                  }
                  onBlur={() =>
                    onChangeCustomResolutionInput(customResolutionInput)
                  }
                  onKeyDown={(event) =>
                    event.key === "Enter" &&
                    onChangeCustomResolutionInput(customResolutionInput)
                  }
                  aria-label="Range"
                  style={{
                    width: `calc(44px + ${customResolutionInput.length + 3}ch)`,
                  }}
                />
              )}
            </Group>

            <SegmentedControl
              onChange={(value) =>
                dispatch(
                  setVisualizer({
                    idx,
                    visualizer: {
                      ...panel.visualizer,
                      displayMode: value as GraphDisplayMode,
                    },
                  })
                )
              }
              value={panel.visualizer.displayMode}
              data={[
                {
                  value: GraphDisplayMode.Lines,
                  label: (
                    <Center>
                      <IconChartLine style={iconStyle} />
                      <Box ml={10}>Unstacked</Box>
                    </Center>
                  ),
                },
                {
                  value: GraphDisplayMode.Stacked,
                  label: (
                    <Center>
                      <IconChartAreaFilled style={iconStyle} />
                      <Box ml={10}>Stacked</Box>
                    </Center>
                  ),
                },
                {
                  value: GraphDisplayMode.Heatmap,
                  label: (
                    <Center>
                      <IconChartGridDots style={iconStyle} />
                      <Box ml={10}>Heatmap</Box>
                    </Center>
                  ),
                },
              ]}
            />
            {/* <Switch color="gray" defaultChecked label="Show exemplars" /> */}
            {/* <Switch
              checked={panel.visualizer.showExemplars}
              onChange={(event) =>
                dispatch(
                  setVisualizer({
                    idx,
                    visualizer: {
                      ...panel.visualizer,
                      showExemplars: event.currentTarget.checked,
                    },
                  })
                )
              }
              color={"rgba(34,139,230,.1)"}
              size="md"
              label="Show exemplars"
              thumbIcon={
                panel.visualizer.showExemplars ? (
                  <IconCheck
                    style={{ width: "0.9rem", height: "0.9rem" }}
                    color={"rgba(34,139,230,.1)"}
                    stroke={3}
                  />
                ) : (
                  <IconX
                    style={{ width: "0.9rem", height: "0.9rem" }}
                    color="rgba(34,139,230,.1)"
                    stroke={3}
                  />
                )
              }
            /> */}
          </Group>
          <Space h="lg" />
          <Graph
            expr={panel.expr}
            endTime={panel.visualizer.endTime}
            range={panel.visualizer.range}
            resolution={panel.visualizer.resolution}
            showExemplars={panel.visualizer.showExemplars}
            displayMode={panel.visualizer.displayMode}
            retriggerIdx={retriggerIdx}
            onSelectRange={(start: number, end: number) =>
              dispatch(
                setVisualizer({
                  idx,
                  visualizer: {
                    ...panel.visualizer,
                    range: (end - start) * 1000,
                    endTime: end * 1000,
                  },
                })
              )
            }
          />
        </Tabs.Panel>
      </Tabs>
    </Stack>
  );
};

export default QueryPanel;
