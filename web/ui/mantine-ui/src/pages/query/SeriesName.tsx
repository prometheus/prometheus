import React, { FC } from "react";
// import { useToastContext } from "../../contexts/ToastContext";
import { formatSeries } from "../../lib/formatSeries";
import classes from "./SeriesName.module.css";
import { escapeString } from "../../lib/escapeString";
import { useClipboard } from "@mantine/hooks";
import { notifications } from "@mantine/notifications";
import {
  maybeQuoteLabelName,
  metricContainsExtendedCharset,
} from "../../promql/utils";

interface SeriesNameProps {
  labels: { [key: string]: string } | null;
  format: boolean;
}

const SeriesName: FC<SeriesNameProps> = ({ labels, format }) => {
  const clipboard = useClipboard();

  const renderFormatted = (): React.ReactElement => {
    const metricExtendedCharset =
      labels && metricContainsExtendedCharset(labels.__name__ || "");

    const labelNodes: React.ReactElement[] = [];
    let first = true;

    // If the metric name uses the extended new charset, we need to escape it,
    // put it into the label matcher list, and make sure it's the first item.
    if (metricExtendedCharset) {
      labelNodes.push(
        <span key="__name__">
          <span className={classes.labelValue}>
            "{escapeString(labels.__name__)}"
          </span>
        </span>
      );

      first = false;
    }

    for (const label in labels) {
      if (label === "__name__") {
        continue;
      }

      labelNodes.push(
        <span key={label}>
          {!first && ", "}
          <span
            className={classes.labelPair}
            onClick={(e) => {
              const text = e.currentTarget.innerText;
              clipboard.copy(text);
              notifications.show({
                title: "Copied matcher!",
                message: `Label matcher ${text} copied to clipboard`,
              });
            }}
            title="Click to copy label matcher"
          >
            <span className={classes.labelName}>
              {maybeQuoteLabelName(label)}
            </span>
            =
            <span className={classes.labelValue}>
              "{escapeString(labels[label])}"
            </span>
          </span>
        </span>
      );

      if (first) {
        first = false;
      }
    }

    return (
      <span>
        {!metricExtendedCharset && (
          <span className={classes.metricName}>
            {labels ? labels.__name__ : ""}
          </span>
        )}
        {"{"}
        {labelNodes}
        {"}"}
      </span>
    );
  };

  if (labels === null) {
    return <>scalar</>;
  }

  if (format) {
    return renderFormatted();
  }
  // Return a simple text node. This is much faster to scroll through
  // for longer lists (hundreds of items).
  return <>{formatSeries(labels)}</>;
};

export default SeriesName;
