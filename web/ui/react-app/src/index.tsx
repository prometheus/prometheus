import './globals';
import React from 'react';
import ReactDOM from 'react-dom';
import App from './App';
import 'bootstrap/dist/css/bootstrap.min.css';
import { isPresent } from './utils';

// Declared/defined in public/index.html, value replaced by Prometheus when serving bundle.
declare const GLOBAL_PATH_PREFIX: string;
declare const GLOBAL_CONSOLES_LINK: string;

let prefix = GLOBAL_PATH_PREFIX;
let consolesLink: string | null = GLOBAL_CONSOLES_LINK;

if (GLOBAL_PATH_PREFIX === 'PATH_PREFIX_PLACEHOLDER' || GLOBAL_PATH_PREFIX === '/' || !isPresent(GLOBAL_PATH_PREFIX)) {
  // Either we are running the app outside of Prometheus, so the placeholder value in
  // the index.html didn't get replaced, or we have a '/' prefix, which we also need to
  // normalize to '' to make concatenations work (prefixes like '/foo/bar/' already get
  // their trailing slash stripped by Prometheus).
  prefix = '';
}

if (
  GLOBAL_CONSOLES_LINK === 'CONSOLES_LINK_PLACEHOLDER' ||
  GLOBAL_CONSOLES_LINK === '' ||
  !isPresent(GLOBAL_CONSOLES_LINK)
) {
  consolesLink = null;
}

ReactDOM.render(<App pathPrefix={prefix} consolesLink={consolesLink} />, document.getElementById('root'));
