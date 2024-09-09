import { FC, PropsWithChildren, useEffect, useState } from "react";
import { useAppDispatch } from "../state/hooks";
import { updateSettings, useSettings } from "../state/settingsSlice";
import { useSuspenseAPIQuery } from "../api/api";
import { WALReplayStatus } from "../api/responseTypes/walreplay";
import { Progress, Stack, Title } from "@mantine/core";
import { useSuspenseQuery } from "@tanstack/react-query";

const ReadinessLoader: FC = () => {
  const { pathPrefix } = useSettings();
  const dispatch = useAppDispatch();

  // Query key is incremented every second to retrigger the status fetching.
  const [queryKey, setQueryKey] = useState(0);

  // Query readiness status.
  const { data: ready } = useSuspenseQuery<boolean>({
    queryKey: ["ready", queryKey],
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    queryFn: async ({ signal }: { signal: AbortSignal }) => {
      try {
        const res = await fetch(`${pathPrefix}/-/ready`, {
          cache: "no-store",
          credentials: "same-origin",
          signal,
        });
        switch (res.status) {
          case 200:
            return true;
          case 503:
            return false;
          default:
            throw new Error(res.statusText);
        }
      } catch (error) {
        throw new Error("Unexpected error while fetching ready status");
      }
    },
  });

  // Query WAL replay status.
  const {
    data: {
      data: { min, max, current },
    },
  } = useSuspenseAPIQuery<WALReplayStatus>({
    path: "/status/walreplay",
    key: ["walreplay", queryKey],
  });

  useEffect(() => {
    if (ready) {
      dispatch(updateSettings({ ready: ready }));
    }
  }, [ready, dispatch]);

  useEffect(() => {
    const interval = setInterval(() => setQueryKey((v) => v + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  return (
    <Stack gap="lg" maw={1000} mx="auto" mt="xs">
      <Title order={2}>Starting up...</Title>
      {max > 0 && (
        <>
          <p>
            Replaying WAL ({current}/{max})
          </p>
          <Progress
            size="xl"
            animated
            value={((current - min + 1) / (max - min + 1)) * 100}
          />
        </>
      )}
    </Stack>
  );
};

export const ReadinessWrapper: FC<PropsWithChildren> = ({ children }) => {
  const { ready } = useSettings();

  if (ready) {
    return <>{children}</>;
  }

  return <ReadinessLoader />;
};

export default ReadinessWrapper;
