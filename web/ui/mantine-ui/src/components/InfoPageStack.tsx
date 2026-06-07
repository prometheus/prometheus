import { Stack } from "@mantine/core";
import { FC, ReactNode } from "react";

const InfoPageStack: FC<{ children: ReactNode }> = ({ children }) => {
  return (
    <Stack gap="lg" maw={1000} mx="auto" mt="xs">
      {children}
    </Stack>
  );
};

export default InfoPageStack;
