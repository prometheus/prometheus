/* eslint-disable prefer-template */
import {component} from 'flightjs';
import $ from 'jquery';
import queryString from 'query-string';

export default component(function lookback() {
  this.refreshCustomFields = function() {
    if (this.$node.val() === 'custom') {
      $('#custom-lookback').show();
    } else {
      $('#custom-lookback').hide();
    }
  };

  this.render = function() {
    const selectedLookback = queryString.parse(window.location.search).lookback;
    this.$node.find('option').each((i, option) => {
      const $option = $(option);
      if ($option.val() === selectedLookback) {
        $option.prop('selected', true);
      }
    });
  };

  this.after('initialize', function() {
    this.render();
    this.refreshCustomFields();

    this.on('change', this.refreshCustomFields);
  });
});
