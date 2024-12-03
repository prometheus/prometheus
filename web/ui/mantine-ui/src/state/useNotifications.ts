import { createContext, useContext } from 'react';
import { Notification } from "../api/responseTypes/notifications";

export type NotificationsContextType = {
  notifications: Notification[];
  isConnectionError: boolean;
};

const defaultContextValue: NotificationsContextType = {
  notifications: [],
  isConnectionError: false,
};

export const NotificationsContext = createContext<NotificationsContextType>(defaultContextValue);

// Custom hook to access notifications context
export const useNotifications = () => useContext(NotificationsContext);
