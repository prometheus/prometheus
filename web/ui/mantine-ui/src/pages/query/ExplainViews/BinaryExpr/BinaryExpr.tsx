import { FC } from "react";
import { BinaryExpr } from "../../../../promql/ast";
import serializeNode from "../../../../promql/serialize";
import VectorScalarBinaryExprExplainView from "./VectorScalar";
import VectorVectorBinaryExprExplainView from "./VectorVector";
import ScalarScalarBinaryExprExplainView from "./ScalarScalar";
import { nodeValueType } from "../../../../promql/utils";
import { useSuspenseAPIQuery } from "../../../../api/api";
import { InstantQueryResult } from "../../../../api/responseTypes/query";
import { Card, Text } from "@mantine/core";

interface BinaryExprExplainViewProps {
  node: BinaryExpr;
}

const BinaryExprExplainView: FC<BinaryExprExplainViewProps> = ({ node }) => {
  const { data: lhs } = useSuspenseAPIQuery<InstantQueryResult>({
    path: `/query`,
    params: {
      query: serializeNode(node.lhs),
    },
  });
  const { data: rhs } = useSuspenseAPIQuery<InstantQueryResult>({
    path: `/query`,
    params: {
      query: serializeNode(node.rhs),
    },
  });

  if (
    lhs.data.resultType !== nodeValueType(node.lhs) ||
    rhs.data.resultType !== nodeValueType(node.rhs)
  ) {
    // This can happen for a brief transitionary render when "node" has changed, but "lhs" and "rhs"
    // haven't switched back to loading yet (leading to a crash in e.g. the vector-vector explain view).
    return null;
  }

  // Scalar-scalar binops.
  if (lhs.data.resultType === "scalar" && rhs.data.resultType === "scalar") {
    return (
      <Card withBorder>
        <Text fz="lg" fw={600} mb="md">
          Scalar-to-scalar binary operation
        </Text>
        <ScalarScalarBinaryExprExplainView
          node={node}
          lhs={lhs.data.result}
          rhs={rhs.data.result}
        />
      </Card>
    );
  }

  // Vector-scalar binops.
  if (lhs.data.resultType === "scalar" && rhs.data.resultType === "vector") {
    return (
      <Card withBorder>
        <Text fz="lg" fw={600} mb="md">
          Scalar-to-vector binary operation
        </Text>
        <VectorScalarBinaryExprExplainView
          node={node}
          vector={rhs.data.result}
          scalar={lhs.data.result}
          scalarLeft={true}
        />
      </Card>
    );
  }
  if (lhs.data.resultType === "vector" && rhs.data.resultType === "scalar") {
    return (
      <Card withBorder>
        <Text fz="lg" fw={600} mb="md">
          Vector-to-scalar binary operation
        </Text>
        <VectorScalarBinaryExprExplainView
          node={node}
          scalar={rhs.data.result}
          vector={lhs.data.result}
          scalarLeft={false}
        />
      </Card>
    );
  }

  // Vector-vector binops.
  if (lhs.data.resultType === "vector" && rhs.data.resultType === "vector") {
    return (
      <Card withBorder>
        <Text fz="lg" fw={600} mb="md">
          Vector-to-vector binary operation
        </Text>
        <VectorVectorBinaryExprExplainView
          node={node}
          lhs={lhs.data.result}
          rhs={rhs.data.result}
        />
      </Card>
    );
  }

  throw new Error("invalid binary operator argument types");
};

export default BinaryExprExplainView;
