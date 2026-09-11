import './globals';
import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import './themes/app.scss';
import './themes/light.scss';
import './themes/dark.scss';
import './fonts/codicon.ttf';
import { isPresent } from './utils';

// Declared/defined in public/index.html, value replaced by Prometheus when serving bundle.
declare const GLOBAL_CONSOLES_LINK: string;
declare const GLOBAL_AGENT_MODE: string;
declare const GLOBAL_READY: string;

let consolesLink: string | null = GLOBAL_CONSOLES_LINK;
const agentMode: string | null = GLOBAL_AGENT_MODE;
const ready: string | null = GLOBAL_READY;

if (
  GLOBAL_CONSOLES_LINK === 'CONSOLES_LINK_PLACEHOLDER' ||
  GLOBAL_CONSOLES_LINK === '' ||
  !isPresent(GLOBAL_CONSOLES_LINK)
) {
  consolesLink = null;
}

createRoot(document.getElementById('root')!).render(
  <App consolesLink={consolesLink} agentMode={agentMode === 'true'} ready={ready === 'true'} />
);
