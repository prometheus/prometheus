import { CodeHighlight } from "@mantine/code-highlight";
import { useSuspenseAPIQuery } from "../api/api";
import ConfigResult from "../api/responseTypes/config";

export default function ConfigPage() {
  const {
    data: {
      data: { yaml },
    },
  } = useSuspenseAPIQuery<ConfigResult>({ path: `/status/config` });

  return (
    <CodeHighlight
      code={yaml}
      language="yaml"
      miw="50vw"
      w="fit-content"
      maw="calc(100vw - 75px)"
      mx="auto"
      mt="xs"
    />
  );
}
