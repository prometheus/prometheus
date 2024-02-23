import { Group, Textarea, Button } from "@mantine/core";
import { IconTerminal } from "@tabler/icons-react";
import { useState } from "react";
import classes from "./graph.module.css";

export default function Graph() {
  const [expr, setExpr] = useState<string>("");

  return (
    <Group align="baseline" wrap="nowrap" gap="xs" mt="sm">
      <Textarea
        style={{ flex: "auto" }}
        classNames={classes}
        placeholder="Enter PromQL expression..."
        value={expr}
        onChange={(event) => setExpr(event.currentTarget.value)}
        leftSection={<IconTerminal />}
        rightSectionPointerEvents="all"
        autosize
        autoFocus
      />
      <Button variant="primary" onClick={() => console.log(expr)}>
        Execute
      </Button>
    </Group>
  );
}
