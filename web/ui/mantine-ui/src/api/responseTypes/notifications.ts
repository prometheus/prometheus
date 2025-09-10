export interface Notification {
  text: string;
  date: string;
  active: boolean;
  modified: boolean;
}

export type NotificationsResult = Notification[];
