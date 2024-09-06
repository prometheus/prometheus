import { FC } from "react";
import { useSuspenseAPIQuery } from "../../api/api";
import { useAppSelector } from "../../state/hooks";
import ASTNode from "../../promql/ast";
import TreeNode from "./TreeNode";
import { Card } from "@mantine/core";

const TreeView: FC<{
  panelIdx: number;
  // TODO: Do we need retriggerIdx for the tree view AST parsing? Maybe for children!
  retriggerIdx: number;
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
}> = ({ panelIdx, selectedNode, setSelectedNode }) => {
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
      <TreeNode
        node={data.data}
        selectedNode={selectedNode}
        setSelectedNode={setSelectedNode}
        reverse={false}
      />
    </Card>
  );
};

export default TreeView;
