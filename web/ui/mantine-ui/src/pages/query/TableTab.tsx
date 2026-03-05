import { FC, useEffect, useId, useLayoutEffect, useState } from "react";
import { Alert, Skeleton, Box, Group, Stack } from "@mantine/core";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { InstantQueryResult } from "../../api/responseTypes/query";
import { useAPIQuery } from "../../api/api";
import dayjs from "dayjs";
import timezone from "dayjs/plugin/timezone";
import { useAppDispatch, useAppSelector } from "../../state/hooks";
import { setVisualizer } from "../../state/queryPageSlice";
import TimeInput from "./TimeInput";
import DataTable from "./DataTable";
import QueryStatsDisplay from "./QueryStatsDisplay";
import { useSettings } from "../../state/settingsSlice";
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
  const { showQueryWarnings, showQueryInfoNotices } = useSettings();

  const { endTime, range } = visualizer;

  const { data, error, isFetching, refetch } = useAPIQuery<InstantQueryResult>({
    key: [useId()],
    path: "/query",
    params: {
      query: expr,
      time: `${(endTime !== null ? endTime : Date.now()) / 1000}`,
      stats: "true",
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
          <QueryStatsDisplay
            numResults={data.data.result.length}
            responseTime={responseTime}
            stats={data.data.stats!}
          />
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

          {showQueryWarnings &&
            data.warnings?.map((w, idx) => (
              <Alert
                key={idx}
                color="red"
                title="Query warning"
                icon={<IconAlertTriangle />}
              >
                {w}
              </Alert>
            ))}

          {showQueryInfoNotices &&
            data.infos?.map((w, idx) => (
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
