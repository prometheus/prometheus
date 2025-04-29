import { Anchor, Badge, Group, Stack } from "@mantine/core";
import { FC } from "react";

export interface EndpointLinkProps {
  endpoint: string;
  globalUrl: string;
}

const EndpointLink: FC<EndpointLinkProps> = ({ endpoint, globalUrl }) => {
  let url: URL;
  let search = "";
  let invalidURL = false;
  try {
    url = new URL(endpoint);
  } catch (_: unknown) {
    // In cases of IPv6 addresses with a Zone ID, URL may not be parseable.
    // See https://github.com/prometheus/prometheus/issues/9760
    // In this case, we attempt to prepare a synthetic URL with the
    // same query parameters, for rendering purposes.
    invalidURL = true;
    if (endpoint.indexOf("?") > -1) {
      search = endpoint.substring(endpoint.indexOf("?"));
    }
    url = new URL("http://0.0.0.0" + search);
  }

  const { host, pathname, protocol, searchParams }: URL = url;
  const params = Array.from(searchParams.entries());
  const displayLink = invalidURL
    ? endpoint.replace(search, "")
    : `${protocol}//${host}${pathname}`;
  return (
    <Stack gap={0}>
      <Anchor size="sm" href={globalUrl} target="_blank">
        {displayLink}
      </Anchor>
      {params.length > 0 && (
        <Group gap="xs" mt="md">
          {params.map(([labelName, labelValue]: [string, string]) => {
            return (
              <Badge
                size="sm"
                variant="light"
                color="gray"
                key={`${labelName}/${labelValue}`}
                style={{ textTransform: "none" }}
              >
                {`${labelName}="${labelValue}"`}
              </Badge>
            );
          })}
        </Group>
      )}
    </Stack>
  );
};

export default EndpointLink;
