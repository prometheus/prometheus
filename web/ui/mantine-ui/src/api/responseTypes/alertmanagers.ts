export type AlertmanagerTarget = {
  url: string;
};

// Result type for /api/v1/alertmanagers endpoint.
// See: https://prometheus.io/docs/prometheus/latest/querying/api/#alertmanagers
export type AlertmanagersResult = {
  activeAlertmanagers: AlertmanagerTarget[];
  droppedAlertmanagers: AlertmanagerTarget[];
};
