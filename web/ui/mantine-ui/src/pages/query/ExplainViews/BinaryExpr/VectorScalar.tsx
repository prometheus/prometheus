import { FC } from "react";
import { BinaryExpr } from "../../../../promql/ast";
// import SeriesName from '../../../../utils/SeriesName';
import { isComparisonOperator } from "../../../../promql/utils";
import { vectorElemBinop } from "../../../../promql/binOp";
import {
  InstantSample,
  SampleValue,
} from "../../../../api/responseTypes/query";
import { Alert, Table, Text } from "@mantine/core";
import {
  formatPrometheusFloat,
  parsePrometheusFloat,
} from "../../../../lib/formatFloatValue";
import SeriesName from "../../SeriesName";

interface VectorScalarBinaryExprExplainViewProps {
  node: BinaryExpr;
  scalar: SampleValue;
  vector: InstantSample[];
  scalarLeft: boolean;
}

const VectorScalarBinaryExprExplainView: FC<
  VectorScalarBinaryExprExplainViewProps
> = ({ node, scalar, vector, scalarLeft }) => {
  if (vector.length === 0) {
    return (
      <Alert>
        One side of the binary operation produces 0 results, no matching
        information shown.
      </Alert>
    );
  }

  return (
    <Table withTableBorder withRowBorders withColumnBorders fz="xs">
      <Table.Thead>
        <Table.Tr>
          {!scalarLeft && <Table.Th>Left labels</Table.Th>}
          <Table.Th>Left value</Table.Th>
          <Table.Th>Operator</Table.Th>
          {scalarLeft && <Table.Th>Right labels</Table.Th>}
          <Table.Th>Right value</Table.Th>
          <Table.Th></Table.Th>
          <Table.Th>Result</Table.Th>
        </Table.Tr>
      </Table.Thead>
      <Table.Tbody>
        {vector.map((sample: InstantSample, idx) => {
          if (!sample.value) {
            // TODO: Handle native histograms or show a better error message.
            throw new Error("Native histograms are not supported yet");
          }

          const vecVal = parsePrometheusFloat(sample.value[1]);
          const scalVal = parsePrometheusFloat(scalar[1]);

          let { value, keep } = scalarLeft
            ? vectorElemBinop(node.op, scalVal, vecVal)
            : vectorElemBinop(node.op, vecVal, scalVal);
          if (isComparisonOperator(node.op) && scalarLeft) {
            value = vecVal;
          }
          if (node.bool) {
            value = Number(keep);
            keep = true;
          }

          const scalarCell = <Table.Td ta="right">{scalar[1]}</Table.Td>;
          const vectorCells = (
            <>
              <Table.Td>
                <SeriesName labels={sample.metric} format={true} />
              </Table.Td>
              <Table.Td ta="right">{sample.value[1]}</Table.Td>
            </>
          );

          return (
            <Table.Tr key={idx}>
              {scalarLeft ? scalarCell : vectorCells}
              <Table.Td ta="center">
                {node.op}
                {node.bool && " bool"}
              </Table.Td>
              {scalarLeft ? vectorCells : scalarCell}
              <Table.Td ta="center">=</Table.Td>
              <Table.Td ta="right">
                {keep ? (
                  formatPrometheusFloat(value)
                ) : (
                  <Text c="dimmed">dropped</Text>
                )}
              </Table.Td>
            </Table.Tr>
          );
        })}
      </Table.Tbody>
    </Table>
  );
};

export default VectorScalarBinaryExprExplainView;
