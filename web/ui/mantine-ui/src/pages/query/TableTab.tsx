import { FC, useEffect, useId, useLayoutEffect, useState } from "react";
import { Alert, Skeleton, Box, Group, Stack, Text } from "@mantine/core";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { InstantQueryResult } from "../../api/responseTypes/query";
import { useAPIQuery } from "../../api/api";
import dayjs from "dayjs";
import timezone from "dayjs/plugin/timezone";
import { useAppDispatch, useAppSelector } from "../../state/hooks";
import { setVisualizer } from "../../state/queryPageSlice";
import TimeInput from "./TimeInput";
import DataTable from "./DataTable";
dayjs.extend(timezone);

export interface TableTabProps {
  panelIdx: number;
  retriggerIdx: number;
  expr: string;
}

const TableTab: FC<TableTabProps> = ({ panelIdx, retriggerIdx, expr }) => {
  const [responseTime, setResponseTime] = useState<number>(0);
  const [limitResults, setLimitResults] = useState<boolean>(true);

  const { visualizer } = useAppSelector(
    (state) => state.queryPage.panels[panelIdx]
  );
  const dispatch = useAppDispatch();

  const { endTime, range } = visualizer;

  const { data, error, isFetching, refetch } = useAPIQuery<InstantQueryResult>({
    key: [useId()],
    path: "/query",
    params: {
      query: expr,
      time: `${(endTime !== null ? endTime : Date.now()) / 1000}`,
    },
    enabled: expr !== "",
    recordResponseTime: setResponseTime,
  });

  useEffect(() => {
    if (expr !== "") {
      refetch();
    }
  }, [retriggerIdx, refetch, expr, endTime]);

  useLayoutEffect(() => {
    setLimitResults(true);
  }, [data, isFetching]);

  return (
    <Stack gap="lg" mt="sm">
      <Group justify="space-between">
        <TimeInput
          time={endTime}
          range={range}
          description="Evaluation time"
          onChangeTime={(time) =>
            dispatch(
              setVisualizer({
                idx: panelIdx,
                visualizer: { ...visualizer, endTime: time },
              })
            )
          }
        />
        {!isFetching && data !== undefined && (
          <Text size="xs" c="gray">
            Load time: {responseTime}ms &ensp; Result series:{" "}
            {data.data.result.length}
          </Text>
        )}
      </Group>
      {isFetching ? (
        <Box>
          {Array.from(Array(5), (_, i) => (
            <Skeleton key={i} height={30} mb={15} />
          ))}
        </Box>
      ) : error !== null ? (
        <Alert
          color="red"
          title="Error executing query"
          icon={<IconAlertTriangle />}
        >
          {error.message}
        </Alert>
      ) : data === undefined ? (
        <Alert variant="transparent">No data queried yet</Alert>
      ) : (
        <>
          {data.data.result.length === 0 && (
            <Alert title="Empty query result" icon={<IconInfoCircle />}>
              This query returned no data.
            </Alert>
          )}

          {data.warnings?.map((w, idx) => (
            <Alert
              key={idx}
              color="red"
              title="Query warning"
              icon={<IconAlertTriangle />}
            >
              {w}
            </Alert>
          ))}

          {data.infos?.map((w, idx) => (
            <Alert
              key={idx}
              color="yellow"
              title="Query notice"
              icon={<IconInfoCircle />}
            >
              {w}
            </Alert>
          ))}
          <DataTable
            data={data.data}
            limitResults={limitResults}
            setLimitResults={setLimitResults}
          />
        </>
      )}
    </Stack>
  );
};

export default TableTab;
