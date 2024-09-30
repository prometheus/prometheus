import React, { useEffect, useState } from 'react';
import { useSettings } from '../state/settingsSlice';
import { NotificationsContext } from '../state/useNotifications';
import { Notification, NotificationsResult } from "../api/responseTypes/notifications";
import { useAPIQuery } from '../api/api';
import { fetchEventSource } from '@microsoft/fetch-event-source';

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
    const controller = new AbortController();
    fetchEventSource(`${pathPrefix}/api/v1/notifications/live`, {
      signal: controller.signal,
      async onopen(response) {
        if (response.ok) {
          if (response.status === 200) {
            setNotifications([]);
            setIsConnectionError(false);
          } else if (response.status === 204) {
            controller.abort();
            setShouldFetchFromAPI(true);
          }
        } else {
          setIsConnectionError(true);
          throw new Error(`Unexpected response: ${response.status} ${response.statusText}`);
        }
      },
      onmessage(event) {
        const notification: Notification = JSON.parse(event.data);

        setNotifications((prev: Notification[]) => {
          const updatedNotifications = [...prev.filter((n: Notification) => n.text !== notification.text)];

          if (notification.active) {
            updatedNotifications.push(notification);
          }

          return updatedNotifications;
        });
      },
      onclose() {
          throw new Error("Server closed the connection");
      },
      onerror() {
        setIsConnectionError(true);
        return 5000;
      },
    });

    return () => {
      controller.abort();
    };
  }, [pathPrefix]);

  return (
    <NotificationsContext.Provider value={{ notifications, isConnectionError }}>
      {children}
    </NotificationsContext.Provider>
  );
};

export default NotificationsProvider;
