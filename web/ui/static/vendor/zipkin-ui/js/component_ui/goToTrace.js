import {component} from 'flightjs';
import {contextRoot} from '../publicPath';

export default component(function goToTrace() {
  this.navigateToTrace = function(evt) {
    evt.preventDefault();
    const traceId = document.getElementById('traceIdQuery').value;
    window.location.href = `${contextRoot}traces/${traceId}`;
  };

  this.after('initialize', function() {
    this.on('submit', this.navigateToTrace);
  });
});
