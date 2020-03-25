import './globals';
import React from 'react';
import ReactDOM from 'react-dom';
import App from './App';
import 'bootstrap/dist/css/bootstrap.min.css';
import { GlobalVarsProvider } from './hooks/useGlobalVars';

ReactDOM.render(
  <GlobalVarsProvider>
    <App />
  </GlobalVarsProvider>,
  document.getElementById('root')
);
