import { Button, Stack } from "@mantine/core";
import { IconPlus } from "@tabler/icons-react";
import { useAppDispatch, useAppSelector } from "../../state/hooks";
import { addPanel } from "../../state/queryPageSlice";
import Panel from "./QueryPanel";

export default function QueryPage() {
  const panels = useAppSelector((state) => state.queryPage.panels);
  const dispatch = useAppDispatch();

  return (
    <>
      <Stack gap="xl">
        {panels.map((p, idx) => (
          <Panel key={p.id} idx={idx} />
        ))}
      </Stack>

      <Button
        variant="light"
        mt="xl"
        leftSection={<IconPlus size={18} />}
        onClick={() => dispatch(addPanel())}
      >
        Add query
      </Button>
    </>
  );
}
