import {
  Group,
  Tabs,
  Center,
  Space,
  Box,
  Input,
  SegmentedControl,
  Stack,
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
  removePanel,
  setExpr,
  setVisualizer,
} from "../../state/queryPageSlice";
import DataTable from "./DataTable";
import TimeInput from "./TimeInput";
import RangeInput from "./RangeInput";
import ExpressionInput from "./ExpressionInput";
import Graph from "./Graph";

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
  const dispatch = useAppDispatch();

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

              <Input value="" placeholder="Res. (s)" style={{ width: 80 }} />
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
          />
        </Tabs.Panel>
      </Tabs>
      {/* Link button to remove this panel. */}
      {/* <Group justify="right">
        <Button
          variant="subtle"
          size="sm"
          fw={500}
          // color="red"
          onClick={() => dispatch(removePanel(idx))}
        >
          Remove query
        </Button>
      </Group> */}
    </Stack>
  );
};

export default QueryPanel;
