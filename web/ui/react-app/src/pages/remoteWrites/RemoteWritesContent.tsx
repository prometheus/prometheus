import { APIResponse } from '../../hooks/useFetch';
import React, { FC, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import { ToggleMoreLess } from '../../components/ToggleMoreLess';
import { Collapse, Table, Badge } from 'reactstrap';
import {formatRelative, humanizeDuration} from "../../utils";
import {now} from "moment";

interface RemoteWriteQueue {
  name: string;
  endpoint: string;

  shardsMax: number;
  shardsMin: number;
  shardsCurrent: number;

  shardingCalculations: ShardingCalculations;
  isResharding: boolean;

  shards: ShardStats[];
}

interface ShardingCalculations {
  lastRan: string;

  delay: number;
  desiredShards: number;
  highestRecv: number;
  highestSent: number;
  samplesInRate: number;
  samplesKeptRatio: number;
  samplesOutDuration: number;
  samplesOutRate: number;
  samplesPending: number;
  samplesPendingRate: number;
  timePerSample: number;
}

interface ShardStats {
  pendingSamples: number;

  lastError: string;
  lastSentTime: string;
  lastSentDuration: number;
}

export interface RemoteWriteRes {
  queues: Array<RemoteWriteQueue>;
}

export interface RemoteWritesContentProps {
  response: APIResponse<RemoteWriteRes>;
}

interface RemoteWritePanelProps {
  queue: RemoteWriteQueue;
}
export const RemoteWritePanel: FC<RemoteWritePanelProps> = ({ queue: q }) => {
  const [showMore, setShowMore] = useState(false);
  const id = `queue-${q.name}`;
  const anchorProps = {
    href: `#${id}`,
    id,
  };

  return (
    <div>
      <h3>
        <ToggleMoreLess
          event={(): void => {
            setShowMore(!showMore);
          }}
          showMore={showMore}
        >
          <a {...anchorProps}>
            {q.name} - {q.endpoint} ({q.shardsMin} min/{q.shardsMax} max/{q.shardsCurrent} current)
          </a>
        </ToggleMoreLess>
      </h3>
      <Collapse isOpen={showMore}>
        <h4>Sharding calculations</h4>
        <p>Data from the calculation last run: {formatRelative(q.shardingCalculations.lastRan, now())}</p>
        <ul>
          <li>
            <strong>Desired shards:</strong> {q.shardingCalculations.desiredShards}
          </li>
          <li>
            <strong>Samples in rate:</strong> {q.shardingCalculations.samplesInRate}
          </li>
          <li>
            <strong>Samples out rate:</strong> {q.shardingCalculations.samplesOutRate}
          </li>
          <li>
            <strong>Samples kept ratio:</strong> {q.shardingCalculations.samplesKeptRatio}
          </li>
          <li>
            <strong>Samples out duration:</strong> {q.shardingCalculations.samplesOutDuration}
          </li>
          <li>
            <strong>Samples pending rate:</strong> {q.shardingCalculations.samplesPendingRate}
          </li>
          <li>
            <strong>Highest Sent:</strong> {q.shardingCalculations.highestSent}
          </li>
          <li>
            <strong>Highest Received:</strong> {q.shardingCalculations.highestRecv}
          </li>
          <li>
            <strong>Delay:</strong> {q.shardingCalculations.delay}
          </li>
        </ul>
        <h4>Shards</h4>
        <Table size="sm" bordered hover striped>
          <thead>
            <tr>
              <th>Shard</th>
              <th>Pending Samples</th>
              <th>Last Send</th>
              <th>Last Send Duration</th>
              <th>Error</th>
            </tr>
          </thead>
          <tbody>
            {q.shards.map((shard, i) => (
              <tr>
                <td>{i}</td>
                <td>{shard.pendingSamples}</td>
                <td>{formatRelative(shard.lastSentTime, now())}</td>
                <td>{humanizeDuration(shard.lastSentDuration * 1000)}</td>
                <td>{shard.lastError ? <Badge color="danger">{shard.lastError}</Badge> : null}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Collapse>
    </div>
  );
};

export const RemoteWritesContent: FC<RouteComponentProps & RemoteWritesContentProps> = ({ response: res }) => {
  // Rough testing data whilst building page out. REMOVE THIS.
  res.data.queues = [
    ...res.data.queues,
    {
      name: 'Secondary Remote',
      endpoint: 'https://an.example.endpoint.com/v2/post',
      shardsMax: 100,
      shardsMin: 1,
      shardsCurrent: 2,
      isResharding: false,
      shardingCalculations: {
        lastRan: '2020-11-22T12:00:00Z',
        delay: 1,
        desiredShards: 12,
        highestRecv: 100.123,
        highestSent: 123.2,
        samplesInRate: 833.123,
        samplesKeptRatio: 12381.23,
        samplesOutDuration: 123.123,
        samplesOutRate: 12,
        samplesPending: 1000,
        samplesPendingRate: 10,
        timePerSample: 15,
      },
      shards: [
        {
          pendingSamples: 120,
          lastSentDuration: 100,
          lastSentTime: '2020-11-22T12:00:00Z',
          lastError: 'Tribbles chewed through the flux capacitor!',
        },
        {
          pendingSamples: 1,
          lastSentDuration: 10,
          lastSentTime: '2020-11-22T12:00:00Z',
          lastError: 'Tribbles chewed through the flux capacitor!',
        },
      ],
    },
  ];
  return (
    <>
      <h2>Remote Writes</h2>
      {res.data.queues.length === 0 ? (
        <p>You have no remote write endpoints configured.</p>
      ) : (
        res.data.queues.map(queue => <RemoteWritePanel queue={queue} />)
      )}
    </>
  );
};
