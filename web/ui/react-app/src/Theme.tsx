import React, { FC, useEffect } from 'react';
import { Form, Button, ButtonGroup } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faMoon, faSun, faAdjust } from '@fortawesome/free-solid-svg-icons';
import { useTheme } from './contexts/ThemeContext';

export const themeLocalStorageKey = 'user-prefers-color-scheme';

export const Theme: FC = () => {
  const { theme } = useTheme();

  useEffect(() => {
    document.body.classList.toggle('bootstrap-dark', theme === 'dark');
    document.body.classList.toggle('bootstrap', theme === 'light');
  }, [theme]);

  return null;
};

export const ThemeToggle: FC = () => {
  const { userPreference, setTheme } = useTheme();

  return (
    <Form className="ml-auto" inline>
      <ButtonGroup size="sm">
        <Button
          color="secondary"
          title="Use light theme"
          active={userPreference === 'light'}
          onClick={() => setTheme('light')}
        >
          <FontAwesomeIcon icon={faSun} className={userPreference === 'light' ? 'text-white' : 'text-dark'} />
        </Button>
        <Button color="secondary" title="Use dark theme" active={userPreference === 'dark'} onClick={() => setTheme('dark')}>
          <FontAwesomeIcon icon={faMoon} className={userPreference === 'dark' ? 'text-white' : 'text-dark'} />
        </Button>
        <Button
          color="secondary"
          title="Use browser-preferred theme"
          active={userPreference === 'auto'}
          onClick={() => setTheme('auto')}
        >
          <FontAwesomeIcon icon={faAdjust} className={userPreference === 'auto' ? 'text-white' : 'text-dark'} />
        </Button>
      </ButtonGroup>
    </Form>
  );
};
