/* eslint-disable no-param-reassign, prefer-template */
import {component} from 'flightjs';
import queryString from 'query-string';
import $ from 'jquery';

// extracted for testing. this code mutates spans and selectedSpans
export function showSpans(spans, parents, children, selectedSpans) {
  const family = new Set();
  $.each(selectedSpans, (i, $selected) => {
    if ($selected.inFilters === 0) {
      $selected.show();
      $selected.addClass('highlight');
    }
    $selected.expanded = true;
    $selected.$expander.text('-');
    $selected.inFilters += 1;

    $.each(children[$selected.id], (j, cId) => {
      family.add(cId);
      spans[cId].openParents += 1;
    });
    $.each(parents[$selected.id], (j, pId) => {
      /* Parent may not be found for a number of reasons. For example, the
      trace id may not be a span id. Also, it is possible to lose the root
      span data (i.e. a headless trace) */
      if (spans[pId]) {
        family.add(pId);
        spans[pId].openChildren += 1;
      }
    });
  });
  family.forEach(id => spans[id].show());
}

function hideSpan(span) {
  if (span.inFilters > 0 || span.openChildren > 0 || span.openParents > 0) return;
  span.hide();
}

// extracted for testing. this code mutates spans and selectedSpans
export function hideSpans(spans, parents, children, selectedSpans, childrenOnly) {
  const family = new Set();
  $.each(selectedSpans, (i, $selected) => {
    $selected.inFilters -= 1;

    if (!childrenOnly && $selected.inFilters === 0) {
      $selected.removeClass('highlight');
      hideSpan($selected);
    }

    $selected.expanded = false;
    $selected.$expander.text('+');

    $.each(children[$selected.id], (j, cId) => {
      family.add(cId);
      spans[cId].openParents -= 1;
    });
    if (!childrenOnly) {
      $.each(parents[$selected.id], (j, pId) => {
        /* Parent may not be found for a number of reasons. For example, the
        trace id may not be a span id. Also, it is possible to lose the root
        span data (i.e. a headless trace) */
        if (spans[pId]) {
          family.add(pId);
          spans[pId].openChildren -= 1;
        }
      });
    }
  });
  family.forEach(id => hideSpan(spans[id]));
}

function spanChildren($span) {
  const children = ($span.attr('data-children') || '').toString().split(',');
  if (children.length === 1 && children[0] === '') {
    $span.find('.expander').hide();
    return [];
  } else {
    return children;
  }
}

function initSpan($span) {
  const id = $span.data('id');
  $span.id = id;
  $span.expanded = false;
  $span.$expander = $span.find('.expander');
  $span.inFilters = 0;
  $span.openChildren = 0;
  $span.openParents = 0;

  const parentId = $span.data('parentId');
  $span.parentId = parentId;
  $span.isRoot = !(parentId !== undefined && parentId !== '');
  return $span;
}

export function initSpans($node) {
  const spans = {};
  const children = {};
  const parents = {};
  const spansByService = {};

  $node.find('.span:not(#timeLabel)').each(function() {
    const span = initSpan($(this));
    const id = span.id;
    const parentId = span.parentId;

    spans[id] = span;
    children[id] = spanChildren(span);
    parents[id] = !span.isRoot ? [parentId] : [];
    $.merge(parents[id], parents[parentId] || []);

    $.each((span.attr('data-service-names') || '').split(','), (i, sn) => {
      const current = spansByService[sn] || [];
      current.push(id);
      spansByService[sn] = current;
    });
  });

  return {
    spans,
    children,
    parents,
    spansByService
  };
}

export default component(function trace() {
  /*
   * Next variables are setting up after initilization.
   * see initSpans
   *
   * this.spans = {};
   * this.parents = {};
   * this.children = {};
   * this.spansByService = {};
   */
  this.spansBackup = {};

  /* this is for a temporary rectangle which is shown on
   * user's mouse move over span view.*/
  this.rectElement = $('<div>').addClass('rect-element');

  /* This method stores original span details for later use.
   * When span view is zoomed in and zoomed out these details help to
   * get back to original span view*/
  this.setupSpansBackup = function($span) {
    const id = $span.data('id');
    $span.id = id;
    this.spansBackup[id] = $span;
  };

  /* Returns a jquery object representing the spans in svc*/
  this.getSpansByService = function(svc) {
    let spans = this.spansByService[svc];
    if (spans === undefined) {
      this.spansByService[svc] = spans = $();
    } else if (spans.jquery === undefined) {
      this.spansByService[svc] = spans = $(`#${spans.join(',#')}`);
    }
    return spans;
  };

  this.filterAdded = function(e, data) {
    if (this.actingOnAll) {
      return;
    }
    const self = this;
    const spans = this.getSpansByService(data.value).map(function() {
      return self.spans[$(this).data('id')];
    });
    this.expandSpans(spans);
  };

  this.expandSpans = function(spans) {
    showSpans(this.spans, this.parents, this.children, spans);
  };

  this.filterRemoved = function(e, data) {
    if (this.actingOnAll) return;
    const self = this;
    const spans = this.getSpansByService(data.value).map(function() {
      return self.spans[$(this).data('id')];
    });
    this.collapseSpans(spans);
  };

  this.collapseSpans = function(spans, childrenOnly) {
    hideSpans(this.spans, this.parents, this.children, spans, childrenOnly);
  };

  this.handleClick = function(e) {
    const $target = $(e.target);
    const $span = this.spans[($target.is('.span') ? $target : $target.parents('.span')).data('id')];

    const $expander = $target.is('.expander') ? $target : $target.parents('.expander');
    if ($expander.length > 0) {
      this.toggleExpander($span);
      return;
    }

    /* incase mouse moved to another span after mousedown, $span is undefined*/
    if ($span && $span.length > 0) {
      this.showSpanDetails($span);
    }
  };

  /* On mousedown and mousemove we need to show a selection area and zoomin
   * spans according to width of selected area. During zoomin only the
   * width i.e. x coordinates are considered.*/
  this.handleMouseDown = function(e) {
    const self = this;

    const rectTop = e.pageY;
    const rectLeft = e.pageX;
    self.rectElement.appendTo(self.$node);

    /* dont draw the rectangle until mouse is moved.
     * this helps in getting the correct parent in case of click
     * event.*/
    self.rectElement.css({top: '0px', left: '0px', width: '0px', height: '0px'});

    self.$node.bind('mousemove', ev => {
      /* prevent selection and thus highlighting of spans after mousedown and mousemove*/
      ev.preventDefault();

      /* draw a rectangle out of mousedown and mousemove coordinates*/
      const rectWidth = Math.abs(ev.pageX - rectLeft);
      const rectHeight = Math.abs(ev.pageY - rectTop);

      const newX = (ev.pageX < rectLeft) ? (rectLeft - rectWidth) : rectLeft;
      const newY = (ev.pageY < rectTop) ? (rectTop - rectHeight) : rectTop;

      self.rectElement.css({top: `${newY}px`, left: `${newX}px`,
          width: `${rectWidth}px`, height: `${rectHeight}px`});
    });

    self.$node.bind('mouseup', ev => {
      self.$node.unbind('mousemove');
      self.$node.unbind('mouseup');
      /* Add code to calculate mintime and max time from pixel value of
       * mouse down and mouse move*/
      const originalDuration = parseFloat($('#timeLabel-backup .time-marker-5').text());
      const spanClickViewLeftOffsetPx = $($('#trace-container .time-marker-0')[1]).offset().left;
      const spanClickViewWidthPx = $('#trace-container .time-marker-5').position().left;

      const rectTopLeft = self.rectElement.position().left;
      /* make sure that redraw mintime starts from 0.0 not less than 0.0.
       * if user starts selecting from servicename adjust the left, width accordingly*/
      const rectElementActualLeft =
        (rectTopLeft < spanClickViewLeftOffsetPx) ? spanClickViewLeftOffsetPx : rectTopLeft;
      const rectElementActualWidth =
        (rectTopLeft < spanClickViewLeftOffsetPx) ?
        (self.rectElement.width() - (spanClickViewLeftOffsetPx - rectTopLeft))
            : self.rectElement.width();

      const minTimeOffsetPx = rectElementActualLeft - spanClickViewLeftOffsetPx;
      const maxTimeOffsetPx = (rectElementActualLeft + rectElementActualWidth)
          - spanClickViewLeftOffsetPx;

      const minTime = minTimeOffsetPx * (originalDuration / spanClickViewWidthPx);
      const maxTime = maxTimeOffsetPx * (originalDuration / spanClickViewWidthPx);

      /* when mousemove doesnt happen mintime is greater than maxtime. since rect-element
       * is created at top left corner of screen, rectElementActualWidth will be negative.
       *We need to invoke mouseclick functionality*/
      if (minTime >= maxTime) {
        /* pass on the target which got clicked. Since we do not draw
         * rectangle on mousedown, we would never endup having
         * rect-element as our target. Target would always be either
         * handle, time-marker, duration which are all children of span class*/
        self.handleClick(ev.target);
      } else {
        /* now that we have min and max time, trigger zoominspans*/
        self.trigger(document, 'uiZoomInSpans', {mintime: minTime, maxtime: maxTime});
      }
      self.rectElement.remove();
    });
  };


  this.toggleExpander = function($span) {
    if ($span.expanded) {
      this.collapseSpans([$span], true);
    } else {
      this.expandSpans([$span], true);
    }
  };

  this.showSpanDetails = function($span) {
    const spanData = {
      annotations: [],
      binaryAnnotations: []
    };

    $.each($span.data('keys').split(','), (i, n) => { spanData[n] = $span.data(n); });

    $span.find('.annotation').each(function() {
      const $this = $(this);
      const anno = {};
      $.each($this.data('keys').split(','), (e, n) => { anno[n] = $this.data(n); });
      spanData.annotations.push(anno);
    });

    $span.find('.binary-annotation').each(function() {
      const $this = $(this);
      const anno = {};
      $.each($this.data('keys').split(','), (e, n) => { anno[n] = $this.data(n); });
      spanData.binaryAnnotations.push(anno);
    });

    this.trigger('uiRequestSpanPanel', spanData);
  };

  this.showSpinnerAround = function(cb, e, data) {
    if (this.actingOnAll) {
      cb(e, data);
    } else {
      this.trigger(document, 'uiShowFullPageSpinner');
      setTimeout(() => {
        cb(e, data);
        this.trigger(document, 'uiHideFullPageSpinner');
      }, 100);
    }
  };

  this.triggerForAllServices = function(evt) {
    $.each(this.spansByService, value => { this.trigger(document, evt, {value}); });
  };

  this.expandAllSpans = function() {
    const self = this;
    self.actingOnAll = true;
    this.showSpinnerAround(() => {
      showSpans(self.spans, self.parents, self.children, self.spans);
      self.triggerForAllServices('uiAddServiceNameFilter');
    });
    self.actingOnAll = false;
  };

  this.collapseAllSpans = function() {
    const self = this;
    self.actingOnAll = true;
    this.showSpinnerAround(() => {
      $.each(self.spans, (id, $span) => {
        $span.inFilters = 0;
        $span.openParents = 0;
        $span.openChildren = 0;
        $span.removeClass('highlight');
        $span.expanded = false;
        $span.$expander.text('+');
        if (!$span.isRoot) $span.hide();
      });
      self.triggerForAllServices('uiRemoveServiceNameFilter');
    });
    self.actingOnAll = false;
  };

  /* This method modifies the span container view. It zooms in the span view
   * for selected time duration. Spans starting with in the selected time
   * duration are highlighted with span name in red color.
   * Also unhides zoomout button so that user can click to go
   * back to original span view*/
  this.zoomInSpans = function(node, data) {
    const self = this;

    const originalDuration = parseFloat($('#timeLabel-backup .time-marker-5').text());

    const mintime = data.mintime;
    const maxtime = data.maxtime;
    const newDuration = maxtime - mintime;

    self.$node.find('#timeLabel .time-marker').each(function(i) {
      const v = (mintime + newDuration * (i / 5)).toFixed(2);
      // TODO:show time units according to new duration
      $(this).text(v);
      $(this).css('color', '#d9534f');
    });

    const styles = {
      left: '0.0%',
      width: '100.0%',
      color: '#000'
    };
    self.showSpinnerAround(() => {
      $.each(self.spans, (id, $span) => {
        /* corresponding to this id extract span from backupspans list*/
        const origLeftVal = parseFloat((self.spansBackup[id]).find('.duration')[0].style.left);
        const origWidthVal = parseFloat((self.spansBackup[id]).find('.duration')[0].style.width);
        /* left and width are in %, so convert them as time values*/
        const spanStart = (origLeftVal * originalDuration) / 100;
        const spanEnd = spanStart + (origWidthVal * originalDuration) / 100;
        /* reset the color to black. It gets set for inrange spans to red*/
        styles.color = '#000';

        /* change style left, width and color of new spans based on mintime and maxtime*/
        if (spanStart < mintime && spanEnd < mintime) {
          styles.left = '0.0%'; styles.width = '0.0%';
        } else if (spanStart < mintime && spanEnd > mintime && spanEnd < maxtime) {
          const w = (((spanEnd - mintime)) / newDuration) * 100 + '%';
          styles.left = '0.0%'; styles.width = w;
        } else if (spanStart < mintime && spanEnd > mintime && spanEnd > maxtime) {
          styles.left = '0.0%'; styles.width = '100.0%';
        } else if (spanStart >= mintime && spanStart < maxtime && spanEnd <= maxtime) {
          const l = (((spanStart - mintime)) / newDuration) * 100 + '%';
          const w = (((spanEnd - spanStart)) / newDuration) * 100 + '%';
          styles.left = l; styles.width = w; styles.color = '#d9534f';
        } else if (spanStart >= mintime && spanStart < maxtime && spanEnd > maxtime) {
          const l = (((spanStart - mintime)) / newDuration) * 100 + '%';
          const w = (((maxtime - spanStart)) / newDuration) * 100 + '%';
          styles.left = l; styles.width = w; styles.color = '#d9534f';
        } else if (spanStart > maxtime) {
          styles.left = '100.0%'; styles.width = '0.0%';
        } else if (spanStart === maxtime) {
          styles.left = '100.0%'; styles.width = '0.0%'; styles.color = '#d9534f';
        } else {
          styles.left = '0.0%'; styles.width = '0.0%';
        }

        $span.find('.duration').css('color', styles.color);
        $span.find('.duration').animate({left: styles.left, width: styles.width}, 'slow');
      });
    });

    /* make sure that zoom-in on already zoomed spanview is not allowed*/
    self.$node.unbind('mousedown');

    /* show zoomOut button now*/
    $('button[value=uiZoomOutSpans]').show('slow');
  };


  /* This method brings back the original span container in view*/
  this.zoomOutSpans = function() {
    /* re bind mousedown event to enable zoom-in after zoom-out*/
    this.on('mousedown', this.handleMouseDown);

    /* get values from the backup trace container*/
    this.$node.find('#timeLabel .time-marker').each(function(i) {
      $(this).css('color', $('#timeLabel-backup .time-marker-' + i).css('color'));
      $(this).text($('#timeLabel-backup .time-marker-' + i).text());
    });

    const self = this;
    this.showSpinnerAround(() => {
      $.each(self.spans, (id, $span) => {
        const originalStyleLeft = (self.spansBackup[id]).find('.duration')[0].style.left;
        const originalStyleWidth = (self.spansBackup[id]).find('.duration')[0].style.width;

        $span.find('.duration').animate({
          left: originalStyleLeft,
          width: originalStyleWidth
        }, 'slow');
        $span.find('.duration').css('color', '#000');
      });
    });

    /* hide zoomOut button now*/
    $('button[value=uiZoomOutSpans]').hide('slow');
  };

  this.after('initialize', function() {
    this.around('filterAdded', this.showSpinnerAround);
    this.around('filterRemoved', this.showSpinnerAround);

    this.on('click', this.handleClick);
    this.on('mousedown', this.handleMouseDown);

    this.on(document, 'uiAddServiceNameFilter', this.filterAdded);
    this.on(document, 'uiRemoveServiceNameFilter', this.filterRemoved);

    this.on(document, 'uiExpandAllSpans', this.expandAllSpans);
    this.on(document, 'uiCollapseAllSpans', this.collapseAllSpans);
    this.on(document, 'uiZoomInSpans', this.zoomInSpans);
    this.on(document, 'uiZoomOutSpans', this.zoomOutSpans);

    const self = this;
    const initData = initSpans(self.$node);
    this.spans = initData.spans;
    this.parents = initData.parents;
    this.children = initData.children;
    this.spansByService = initData.spansByService;

    /* get spans from trace-container-backup*/
    $('#trace-container-backup .span:not(#timeLabel-backup)').each(function() {
      self.setupSpansBackup($(this));
    });

    const serviceName = queryString.parse(location.search).serviceName;
    if (serviceName !== undefined) {
      this.trigger(document, 'uiAddServiceNameFilter', {value: serviceName});
    } else {
      this.trigger(document, 'uiExpandAllSpans');
    }
  });
});
