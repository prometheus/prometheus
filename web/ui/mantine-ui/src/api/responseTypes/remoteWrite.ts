export interface QueueStatus {
  name: string;
  endpoint: string;

  highestTimestampSec: number;
  highestSentTimestampSec: number;

  pendingSamples: number;
  pendingExemplars: number;
  pendingHistograms: number;

  currentShards: number;
  desiredShards: number;
  minShards: number;
  maxShards: number;
  shardCapacity: number;

  samplesSent: number;
  exemplarsSent: number;
  histogramsSent: number;
  metadataSent: number;
  bytesSent: number;

  samplesFailed: number;
  exemplarsFailed: number;
  histogramsFailed: number;
  metadataFailed: number;

  samplesRetried: number;
  exemplarsRetried: number;
  histogramsRetried: number;
  metadataRetried: number;
}

export interface RemoteWriteStatus {
  queues: QueueStatus[];
}
