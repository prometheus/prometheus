import './globals';
import React from 'react';
import ReactDOM from 'react-dom';
import App from './App';
import 'bootstrap/dist/css/bootstrap.min.css';
import { isPresent } from './utils';

// Declared/defined in public/index.html, value replaced by Prometheus when serving bundle.
declare const GLOBAL_CONSOLES_LINK: string;

let consolesLink: string | null = GLOBAL_CONSOLES_LINK;

if (
  GLOBAL_CONSOLES_LINK === 'CONSOLES_LINK_PLACEHOLDER' ||
  GLOBAL_CONSOLES_LINK === '' ||
  !isPresent(GLOBAL_CONSOLES_LINK)
) {
  consolesLink = null;
}

ReactDOM.render(<App consolesLink={consolesLink} />, document.getElementById('root'));
