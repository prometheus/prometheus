import {contextRoot} from './publicPath';
import $ from 'jquery';

const defaults = {
  environment: '',
  queryLimit: 10,
  defaultLookback: 60 * 60 * 1000, // 1 hour
  searchEnabled: true,
};

export default function loadConfig() {
  return $.ajax(`${contextRoot}config.json`, {
    type: 'GET',
    dataType: 'json'
  }).then(data => function config(key) {
    if (data[key] === false) {
      return false;
    }

    return data[key] || defaults[key];
  });
}
