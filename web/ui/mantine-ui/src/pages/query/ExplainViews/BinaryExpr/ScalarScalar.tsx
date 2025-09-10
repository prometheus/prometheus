import { FC } from "react";
import { BinaryExpr } from "../../../../promql/ast";
import { scalarBinOp } from "../../../../promql/binOp";
import { Table } from "@mantine/core";
import { SampleValue } from "../../../../api/responseTypes/query";
import {
  formatPrometheusFloat,
  parsePrometheusFloat,
} from "../../../../lib/formatFloatValue";

interface ScalarScalarBinaryExprExplainViewProps {
  node: BinaryExpr;
  lhs: SampleValue;
  rhs: SampleValue;
}

const ScalarScalarBinaryExprExplainView: FC<
  ScalarScalarBinaryExprExplainViewProps
> = ({ node, lhs, rhs }) => {
  const [lhsVal, rhsVal] = [
    parsePrometheusFloat(lhs[1]),
    parsePrometheusFloat(rhs[1]),
  ];

  return (
    <Table withColumnBorders withTableBorder>
      <Table.Thead>
        <Table.Tr>
          <Table.Th>Left value</Table.Th>
          <Table.Th>Operator</Table.Th>
          <Table.Th>Right value</Table.Th>
          <Table.Th></Table.Th>
          <Table.Th>Result</Table.Th>
        </Table.Tr>
      </Table.Thead>
      <Table.Tbody>
        <Table.Tr>
          <Table.Td className="number-cell">{lhs[1]}</Table.Td>
          <Table.Td className="op-cell">
            {node.op}
            {node.bool && " bool"}
          </Table.Td>
          <Table.Td className="number-cell">{rhs[1]}</Table.Td>
          <Table.Td className="op-cell">=</Table.Td>
          <Table.Td className="number-cell">
            {formatPrometheusFloat(scalarBinOp(node.op, lhsVal, rhsVal))}
          </Table.Td>
        </Table.Tr>
      </Table.Tbody>
    </Table>
  );
};

export default ScalarScalarBinaryExprExplainView;
