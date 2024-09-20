import React, { useEffect, useState } from 'react';
import { useSettings } from '../state/settingsSlice';
import { NotificationsContext } from '../state/useNotifications';
import { Notification, NotificationsResult } from "../api/responseTypes/notifications";
import { useAPIQuery } from '../api/api';

export const NotificationsProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { pathPrefix } = useSettings();
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [isConnectionError, setIsConnectionError] = useState(false);
  const [shouldFetchFromAPI, setShouldFetchFromAPI] = useState(false);

  const { data, isError } = useAPIQuery<NotificationsResult>({
    path: '/notifications',
    enabled: shouldFetchFromAPI,
    refetchInterval: 10000,
  });

  useEffect(() => {
    if (data && data.data) {
      setNotifications(data.data);
    }
    setIsConnectionError(isError);
  }, [data, isError]);

  useEffect(() => {
    const eventSource = new EventSource(`${pathPrefix}/api/v1/notifications/live`);

    eventSource.onmessage = (event) => {
      const notification: Notification = JSON.parse(event.data);

      setNotifications((prev: Notification[]) => {
        const updatedNotifications = [...prev.filter((n: Notification) => n.text !== notification.text)];

        if (notification.active) {
          updatedNotifications.push(notification);
        }

        return updatedNotifications;
      });
    };

    eventSource.onerror = () => {
      eventSource.close();
      setIsConnectionError(true);
      setShouldFetchFromAPI(true);
    };

    return () => {
      eventSource.close();
    };
  }, [pathPrefix]);

  return (
    <NotificationsContext.Provider value={{ notifications, isConnectionError }}>
      {children}
    </NotificationsContext.Provider>
  );
};

export default NotificationsProvider;
