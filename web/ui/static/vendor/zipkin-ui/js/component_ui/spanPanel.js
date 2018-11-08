import {component} from 'flightjs';
import $ from 'jquery';
import {Constants} from './traceConstants';

const entityMap = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
  '/': '&#x2F;',
  '`': '&#x60;',
  '=': '&#x3D;'
};

function escapeHtml(string) {
  return String(string).replace(/[&<>"'`=\/]/g, s => entityMap[s]);
}

export function isDupeBinaryAnnotation(tagMap, anno) {
  if (!tagMap[anno.key]) {
    tagMap[anno.key] = anno.value; // eslint-disable-line no-param-reassign
  } else if (tagMap[anno.key] === anno.value) {
    return true;
  }
  return false;
}

// Annotation values that contain the word "error" hint of a transient error.
// This adds a class when that's the case.
export function maybeMarkTransientError(row, anno) {
  if (/error/i.test(anno.value)) {
    row.addClass('anno-error-transient');
  }
}

// Normal values are formatted in traceToMustache. However, Quoted json values
// end up becoming javascript objects later. For this reason, we have to guard
// and stringify as necessary.

// annotations are named events which shouldn't hold json. If someone passed
// json, format as a single line. That way the rows corresponding to timestamps
// aren't disrupted.
export function formatAnnotationValue(value) {
  const type = $.type(value);
  if (type === 'object' || type === 'array' || value == null) {
    return escapeHtml(JSON.stringify(value));
  } else {
    return escapeHtml(value.toString()); // prevents false from coercing to empty!
  }
}

// Binary annotations are tags, and sometimes the values are large, for example
// json representing a query or a stack trace. Format these so that they don't
// scroll off the side of the screen.
export function formatBinaryAnnotationValue(value) {
  const type = $.type(value);
  if (type === 'object' || type === 'array' || value == null) {
    return `<pre><code>${escapeHtml(JSON.stringify(value, null, 2))}</code></pre>`;
  }
  const result = value.toString();
  // Preformat if the text includes newlines
  return result.indexOf('\n') === -1 ? escapeHtml(result)
    : `<pre><code>${escapeHtml(result)}</code></pre>`;
}

export default component(function spanPanel() {
  this.$annotationTemplate = null;
  this.$binaryAnnotationTemplate = null;
  this.$moreInfoTemplate = null;

  this.show = function(e, span) {
    const self = this;
    const tagMap = {};

    this.$node.find('.modal-title').text(
      `${span.serviceName}.${span.spanName}: ${span.durationStr}`);

    this.$node.find('.service-names').text(span.serviceNames);

    const $annoBody = this.$node.find('#annotations tbody').text('');
    $.each((span.annotations || []), (i, anno) => {
      const $row = self.$annotationTemplate.clone();
      maybeMarkTransientError($row, anno);
      $row.find('td').each(function() {
        const $this = $(this);
        const propertyName = $this.data('key');
        const text = propertyName === 'value'
          ? formatAnnotationValue(anno.value)
          : anno[propertyName];
        $this.append(text);
      });
      $annoBody.append($row);
    });

    $annoBody.find('.local-datetime').each(function() {
      const $this = $(this);
      const timestamp = $this.text();
      $this.text((new Date(parseInt(timestamp, 10) / 1000)).toLocaleString());
    });

    const $binAnnoBody = this.$node.find('#binaryAnnotations tbody').text('');
    $.each((span.binaryAnnotations || []), (i, anno) => {
      if (isDupeBinaryAnnotation(tagMap, anno)) return;
      const $row = self.$binaryAnnotationTemplate.clone();
      if (anno.key === Constants.ERROR) {
        $row.addClass('anno-error-critical');
      }
      $row.find('td').each(function() {
        const $this = $(this);
        const propertyName = $this.data('key');
        const text = propertyName === 'value'
          ? formatBinaryAnnotationValue(anno.value)
          : escapeHtml(anno[propertyName]);
        $this.append(text);
      });
      $binAnnoBody.append($row);
    });

    const $moreInfoBody = this.$node.find('#moreInfo tbody').text('');
    const moreInfo = [['traceId', span.traceId],
                      ['spanId', span.id],
                      ['parentId', span.parentId]];
    $.each(moreInfo, (i, pair) => {
      const $row = self.$moreInfoTemplate.clone();
      $row.find('.key').text(pair[0]);
      $row.find('.value').text(pair[1]);
      $moreInfoBody.append($row);
    });

    this.$node.modal('show');
  };

  this.after('initialize', function() {
    this.$node.modal('hide');
    this.$annotationTemplate = this.$node.find('#annotations tbody tr').remove();
    this.$binaryAnnotationTemplate = this.$node.find('#binaryAnnotations tbody tr').remove();
    this.$moreInfoTemplate = this.$node.find('#moreInfo tbody tr').remove();
    this.on(document, 'uiRequestSpanPanel', this.show);
  });
});
