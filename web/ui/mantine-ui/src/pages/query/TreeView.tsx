import { FC } from "react";
import { useSuspenseAPIQuery } from "../../api/api";
import { useAppSelector } from "../../state/hooks";
import ASTNode from "../../promql/ast";
import TreeNode from "./TreeNode";
import { Card, CloseButton } from "@mantine/core";

const TreeView: FC<{
  panelIdx: number;
  selectedNode: {
    id: string;
    node: ASTNode;
  } | null;
  setSelectedNode: (
    node: {
      id: string;
      node: ASTNode;
    } | null
  ) => void;
  closeTreeView: () => void;
}> = ({ panelIdx, selectedNode, setSelectedNode, closeTreeView }) => {
  const { expr } = useAppSelector((state) => state.queryPage.panels[panelIdx]);

  const { data } = useSuspenseAPIQuery<ASTNode>({
    path: "/parse_query",
    params: {
      query: expr,
    },
    enabled: expr !== "",
  });

  return (
    <Card withBorder fz="sm" style={{ overflowX: "auto" }} pl="sm">
      <CloseButton
        aria-label="Close tree view"
        title="Close tree view"
        pos="absolute"
        top={7}
        size="sm"
        right={7}
        onClick={closeTreeView}
      />
      <TreeNode
        childIdx={0}
        node={data.data}
        selectedNode={selectedNode}
        setSelectedNode={setSelectedNode}
        reverse={false}
      />
    </Card>
  );
};

export default TreeView;
