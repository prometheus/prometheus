/**
 * version 1.0.4
 * @license
 * =========================================================
 * bootstrap-datetimepicker.js
 * http://www.eyecon.ro/bootstrap-datepicker
 * =========================================================
 * Copyright 2012 Stefan Petre
 *
 * Contributions:
 *  - Andrew Rowls
 *  - Thiago de Arruda
 *  - updated for Bootstrap v3 by Jonathan Peterson @Eonasdan
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 * =========================================================
 */

(function ($) {

    // Picker object
    var smartPhone = (window.orientation !== undefined);
    var DateTimePicker = function (element, options) {
        this.id = dpgId++;
        this.init(element, options);
    };

    var dateToDate = function (dt) {
        if (typeof dt === 'string') {
            return new Date(dt);
        }
        return dt;
    };

    DateTimePicker.prototype = {
        constructor: DateTimePicker,

        init: function (element, options) {
            var icon = false;
            if (!(options.pickTime || options.pickDate))
                throw new Error('Must choose at least one picker');
            this.options = options;
            this.$element = $(element);
            this.language = options.language in dates ? options.language : 'en';
            this.pickDate = options.pickDate;
            this.pickTime = options.pickTime;
            this.isInput = this.$element.is('input');
            this.component = false;
            if (this.$element.hasClass('input-group'))
                this.component = this.$element.find('.input-group-addon');
            this.format = options.format;
            if (!this.format) {
				if (dates[this.language].format != null)  this.format = dates[this.language].format;
                else if (this.isInput) this.format = this.$element.data('format');
                else this.format = this.$element.find('input').data('format');
                if (!this.format) this.format = (this.pickDate ? 'MM/dd/yyyy' : '')
				this.format += (this.pickTime ? ' hh:mm' : '') + (this.pickSeconds ? ':ss' : '');
            }
            this._compileFormat();
            if (this.component) icon = this.component.find('span');

            if (this.pickTime) {
                if (icon && icon.length) {
                  this.timeIcon = icon.data('time-icon');
                  this.upIcon = icon.data('up-icon');
                  this.downIcon = icon.data('down-icon');
                }
                if (!this.timeIcon) this.timeIcon = 'glyphicon glyphicon-time';
                if (!this.upIcon) this.upIcon = 'glyphicon glyphicon-chevron-up';
                if (!this.downIcon) this.downIcon = 'glyphicon glyphicon-chevron-down';
				if (icon) icon.addClass(this.timeIcon);
            }
            if (this.pickDate) {
                if (icon && icon.length) this.dateIcon = icon.data('date-icon');
                if (!this.dateIcon) this.dateIcon = 'glyphicon glyphicon-calendar';
				if (icon) {
					icon.removeClass(this.timeIcon);
					icon.addClass(this.dateIcon);
				}
            }
            this.widget = $(getTemplate(this.timeIcon, this.upIcon, this.downIcon, options.pickDate, options.pickTime,
                                        options.pick12HourFormat, options.pickSeconds, options.collapse)).appendTo('body');
            this.minViewMode = options.minViewMode || this.$element.data('date-minviewmode') || 0;
            if (typeof this.minViewMode === 'string') {
                switch (this.minViewMode) {
                    case 'months':
                        this.minViewMode = 1;
                        break;
                    case 'years':
                        this.minViewMode = 2;
                        break;
                    default:
                        this.minViewMode = 0;
                        break;
                }
            }
            this.viewMode = options.viewMode || this.$element.data('date-viewmode') || 0;
            if (typeof this.viewMode === 'string') {
                switch (this.viewMode) {
                    case 'months':
                        this.viewMode = 1;
                        break;
                    case 'years':
                        this.viewMode = 2;
                        break;
                    default:
                        this.viewMode = 0;
                        break;
                }
            }
			this.startViewMode = this.viewMode;
            this.weekStart = options.weekStart || this.$element.data('date-weekstart') || 0;
            this.weekEnd = this.weekStart === 0 ? 6 : this.weekStart - 1;
            this.setStartDate(options.startDate || this.$element.data('date-startdate'));
            this.setEndDate(options.endDate || this.$element.data('date-enddate'));
            this.fillDow();
            this.fillMonths();
            this.fillHours();
            this.fillMinutes();
            this.fillSeconds();
            this.update();
            this.showMode();
            this._attachDatePickerEvents();
        },

        show: function (e) {
            this.widget.show();
            this.height = this.component ? this.component.outerHeight() : this.$element.outerHeight();
            this.place();
            this.$element.trigger({
                type: 'show',
                date: this._date
            });
            this._attachDatePickerGlobalEvents();
            if (e) {
                e.stopPropagation();
                e.preventDefault();
            }
        },

        disable: function () {
            this.$element.find('input').prop('disabled', true);
            this._detachDatePickerEvents();
        },
        enable: function () {
            this.$element.find('input').prop('disabled', false);
            this._attachDatePickerEvents();
        },

        hide: function () {
            // Ignore event if in the middle of a picker transition
            var collapse = this.widget.find('.collapse');
            for (var i = 0; i < collapse.length; i++) {
                var collapseData = collapse.eq(i).data('collapse');
                if (collapseData && collapseData.transitioning)
                    return;
            }
            this.widget.hide();
            this.viewMode = this.startViewMode;
            this.showMode();
            this.$element.trigger({
                type: 'hide',
                date: this._date
            });
            this._detachDatePickerGlobalEvents();
        },

        set: function () {
            var formatted = '';
            if (!this._unset) formatted = this.formatDate(this._date);
            if (!this.isInput) {
                if (this.component) {
                    var input = this.$element.find('input');
                    input.val(formatted);
                    this._resetMaskPos(input);
                }
                this.$element.data('date', formatted);
            } else {
                this.$element.val(formatted);
                this._resetMaskPos(this.$element);
            }
            if (!this.pickTime) this.hide();
        },

        setValue: function (newDate) {
            if (!newDate) {
                this._unset = true;
            } else {
                this._unset = false;
            }
            if (typeof newDate === 'string') {
                this._date = this.parseDate(newDate);
            } else if (newDate) {
                this._date = new Date(newDate);
            }
            this.set();
            this.viewDate = new UTCDate(this._date.getUTCFullYear(), this._date.getUTCMonth(), 1, 0, 0, 0, 0);
            this.fillDate();
            this.fillTime();
        },

        getDate: function () {
            if (this._unset) return null;
            return new Date(this._date.valueOf());
        },

        setDate: function (date) {
            if (!date) this.setValue(null);
            else this.setValue(date.valueOf());
        },

        setStartDate: function (date) {
            if (date instanceof Date) {
                this.startDate = date;
            } else if (typeof date === 'string') {
                this.startDate = new UTCDate(date);
                if (!this.startDate.getUTCFullYear()) {
                    this.startDate = -Infinity;
                }
            } else {
                this.startDate = -Infinity;
            }
            if (this.viewDate) {
                this.update();
            }
        },

        setEndDate: function (date) {
            if (date instanceof Date) {
                this.endDate = date;
            } else if (typeof date === 'string') {
                this.endDate = new UTCDate(date);
                if (!this.endDate.getUTCFullYear()) {
                    this.endDate = Infinity;
                }
            } else {
                this.endDate = Infinity;
            }
            if (this.viewDate) {
                this.update();
            }
        },

        getLocalDate: function () {
            if (this._unset) return null;
            var d = this._date;
            return new Date(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate(),
                            d.getUTCHours(), d.getUTCMinutes(), d.getUTCSeconds(), d.getUTCMilliseconds());
        },

        setLocalDate: function (localDate) {
            if (!localDate) this.setValue(null);
            else
                this.setValue(Date.UTC(
                  localDate.getFullYear(),
                  localDate.getMonth(),
                  localDate.getDate(),
                  localDate.getHours(),
                  localDate.getMinutes(),
                  localDate.getSeconds(),
                  localDate.getMilliseconds()));
        },

        place: function () {
            var position = 'absolute';
            var offset = this.component ? this.component.offset() : this.$element.offset();
            this.width = this.component ? this.component.outerWidth() : this.$element.outerWidth();
            offset.top = offset.top + this.height;

            var $window = $(window);

            if (this.options.width !== undefined) {
                this.widget.width(this.options.width);
            }

            if (this.options.orientation === 'left') {
                this.widget.addClass('left-oriented');
                offset.left = offset.left - this.widget.width() + 20;
            }

            if (this._isInFixed()) {
                position = 'fixed';
                offset.top -= $window.scrollTop();
                offset.left -= $window.scrollLeft();
            }

            if ($window.width() < offset.left + this.widget.outerWidth()) {
                offset.right = $window.width() - offset.left - this.width;
                offset.left = 'auto';
                this.widget.addClass('pull-right');
            } else {
                offset.right = 'auto';
                this.widget.removeClass('pull-right');
            }

            this.widget.css({
                position: position,
                top: offset.top,
                left: offset.left,
                right: offset.right
            });
        },

        notifyChange: function () {
            this.$element.trigger({
                type: 'changeDate',
                date: this.getDate(),
                localDate: this.getLocalDate()
            });
        },

        update: function (newDate) {
            var dateStr = newDate;
            if (!dateStr) {
                if (this.isInput) {
                    dateStr = this.$element.val();
                } else {
                    dateStr = this.$element.find('input').val();
                }
                if (dateStr) {
                    this._date = this.parseDate(dateStr);
                }
                if (!this._date) {
                    var tmp = new Date();
                    this._date = new UTCDate(tmp.getFullYear(),
                                        tmp.getMonth(),
                                        tmp.getDate(),
                                        tmp.getHours(),
                                        tmp.getMinutes(),
                                        tmp.getSeconds(),
                                        tmp.getMilliseconds());
                }
            }
            this.viewDate = new UTCDate(this._date.getUTCFullYear(), this._date.getUTCMonth(), 1, 0, 0, 0, 0);
            this.fillDate();
            this.fillTime();
        },

        fillDow: function () {
            var dowCnt = this.weekStart;
            var html = $('<tr>');
            while (dowCnt < this.weekStart + 7) {
                html.append('<th class="dow">' + dates[this.language].daysMin[(dowCnt++) % 7] + '</th>');
            }
            this.widget.find('.datepicker-days thead').append(html);
        },

        fillMonths: function () {
            var html = '';
            var i = 0;
            while (i < 12) {
                html += '<span class="month">' + dates[this.language].monthsShort[i++] + '</span>';
            }
            this.widget.find('.datepicker-months td').append(html);
        },

        fillDate: function () {
            var year = this.viewDate.getUTCFullYear();
            var month = this.viewDate.getUTCMonth();
            var currentDate = UTCDate(
              this._date.getUTCFullYear(),
              this._date.getUTCMonth(),
              this._date.getUTCDate(),
              0, 0, 0, 0
            );
            var startYear = typeof this.startDate === 'object' ? this.startDate.getUTCFullYear() : -Infinity;
            var startMonth = typeof this.startDate === 'object' ? this.startDate.getUTCMonth() : -1;
            var endYear = typeof this.endDate === 'object' ? this.endDate.getUTCFullYear() : Infinity;
            var endMonth = typeof this.endDate === 'object' ? this.endDate.getUTCMonth() : 12;

            this.widget.find('.datepicker-days').find('.disabled').removeClass('disabled');
            this.widget.find('.datepicker-months').find('.disabled').removeClass('disabled');
            this.widget.find('.datepicker-years').find('.disabled').removeClass('disabled');

            this.widget.find('.datepicker-days th:eq(1)').text(
              dates[this.language].months[month] + ' ' + year);

            var prevMonth = UTCDate(year, month - 1, 28, 0, 0, 0, 0);
            var day = DPGlobal.getDaysInMonth(
              prevMonth.getUTCFullYear(), prevMonth.getUTCMonth());
            prevMonth.setUTCDate(day);
            prevMonth.setUTCDate(day - (prevMonth.getUTCDay() - this.weekStart + 7) % 7);
            if ((year == startYear && month <= startMonth) || year < startYear) {
                this.widget.find('.datepicker-days th:eq(0)').addClass('disabled');
            }
            if ((year == endYear && month >= endMonth) || year > endYear) {
                this.widget.find('.datepicker-days th:eq(2)').addClass('disabled');
            }

            var nextMonth = new Date(prevMonth.valueOf());
            nextMonth.setUTCDate(nextMonth.getUTCDate() + 42);
            nextMonth = nextMonth.valueOf();
            var html = [];
            var row;
            var clsName;
            while (prevMonth.valueOf() < nextMonth) {
                if (prevMonth.getUTCDay() === this.weekStart) {
                    row = $('<tr>');
                    html.push(row);
                }
                clsName = '';
                if (prevMonth.getUTCFullYear() < year ||
                    (prevMonth.getUTCFullYear() == year &&
                     prevMonth.getUTCMonth() < month)) {
                    clsName += ' old';
                } else if (prevMonth.getUTCFullYear() > year ||
                           (prevMonth.getUTCFullYear() == year &&
                            prevMonth.getUTCMonth() > month)) {
                    clsName += ' new';
                }
                if (prevMonth.valueOf() === currentDate.valueOf()) {
                    clsName += ' active';
                }
                if ((prevMonth.valueOf() + 86400000) <= this.startDate) {
                    clsName += ' disabled';
                }
                if (prevMonth.valueOf() > this.endDate) {
                    clsName += ' disabled';
                }
                row.append('<td class="day' + clsName + '">' + prevMonth.getUTCDate() + '</td>');
                prevMonth.setUTCDate(prevMonth.getUTCDate() + 1);
            }
            this.widget.find('.datepicker-days tbody').empty().append(html);
            var currentYear = this._date.getUTCFullYear();

            var months = this.widget.find('.datepicker-months').find(
              'th:eq(1)').text(year).end().find('span').removeClass('active');
            if (currentYear === year) {
                months.eq(this._date.getUTCMonth()).addClass('active');
            }
            if (currentYear - 1 < startYear) {
                this.widget.find('.datepicker-months th:eq(0)').addClass('disabled');
            }
            if (currentYear + 1 > endYear) {
                this.widget.find('.datepicker-months th:eq(2)').addClass('disabled');
            }
            for (var i = 0; i < 12; i++) {
                if ((year == startYear && startMonth > i) || (year < startYear)) {
                    $(months[i]).addClass('disabled');
                } else if ((year == endYear && endMonth < i) || (year > endYear)) {
                    $(months[i]).addClass('disabled');
                }
            }

            html = '';
            year = parseInt(year / 10, 10) * 10;
            var yearCont = this.widget.find('.datepicker-years').find(
              'th:eq(1)').text(year + '-' + (year + 9)).end().find('td');
            this.widget.find('.datepicker-years').find('th').removeClass('disabled');
            if (startYear > year) {
                this.widget.find('.datepicker-years').find('th:eq(0)').addClass('disabled');
            }
            if (endYear < year + 9) {
                this.widget.find('.datepicker-years').find('th:eq(2)').addClass('disabled');
            }
            year -= 1;
            for (var i = -1; i < 11; i++) {
                html += '<span class="year' + (i === -1 || i === 10 ? ' old' : '') + (currentYear === year ? ' active' : '') + ((year < startYear || year > endYear) ? ' disabled' : '') + '">' + year + '</span>';
                year += 1;
            }
            yearCont.html(html);
        },

        fillHours: function () {
            var table = this.widget.find(
              '.timepicker .timepicker-hours table');
            table.parent().hide();
            var html = '';
            if (this.options.pick12HourFormat) {
                var current = 1;
                for (var i = 0; i < 3; i += 1) {
                    html += '<tr>';
                    for (var j = 0; j < 4; j += 1) {
                        var c = current.toString();
                        html += '<td class="hour">' + padLeft(c, 2, '0') + '</td>';
                        current++;
                    }
                    html += '</tr>';
                }
            } else {
                var current = 0;
                for (var i = 0; i < 6; i += 1) {
                    html += '<tr>';
                    for (var j = 0; j < 4; j += 1) {
                        var c = current.toString();
                        html += '<td class="hour">' + padLeft(c, 2, '0') + '</td>';
                        current++;
                    }
                    html += '</tr>';
                }
            }
            table.html(html);
        },

        fillMinutes: function () {
            var table = this.widget.find(
              '.timepicker .timepicker-minutes table');
            table.parent().hide();
            var html = '';
            var current = 0;
            for (var i = 0; i < 5; i++) {
                html += '<tr>';
                for (var j = 0; j < 4; j += 1) {
                    var c = current.toString();
                    html += '<td class="minute">' + padLeft(c, 2, '0') + '</td>';
                    current += 3;
                }
                html += '</tr>';
            }
            table.html(html);
        },

        fillSeconds: function () {
            var table = this.widget.find(
              '.timepicker .timepicker-seconds table');
            table.parent().hide();
            var html = '';
            var current = 0;
            for (var i = 0; i < 5; i++) {
                html += '<tr>';
                for (var j = 0; j < 4; j += 1) {
                    var c = current.toString();
                    html += '<td class="second">' + padLeft(c, 2, '0') + '</td>';
                    current += 3;
                }
                html += '</tr>';
            }
            table.html(html);
        },

        fillTime: function () {
            if (!this._date)
                return;
            var timeComponents = this.widget.find('.timepicker span[data-time-component]');
            var table = timeComponents.closest('table');
            var is12HourFormat = this.options.pick12HourFormat;
            var hour = this._date.getUTCHours();
            var period = 'AM';
            if (is12HourFormat) {
                if (hour >= 12) period = 'PM';
                if (hour === 0) hour = 12;
                else if (hour != 12) hour = hour % 12;
                this.widget.find(
                  '.timepicker [data-action=togglePeriod]').text(period);
            }
            hour = padLeft(hour.toString(), 2, '0');
            var minute = padLeft(this._date.getUTCMinutes().toString(), 2, '0');
            var second = padLeft(this._date.getUTCSeconds().toString(), 2, '0');
            timeComponents.filter('[data-time-component=hours]').text(hour);
            timeComponents.filter('[data-time-component=minutes]').text(minute);
            timeComponents.filter('[data-time-component=seconds]').text(second);
        },

        click: function (e) {
            e.stopPropagation();
            e.preventDefault();
            this._unset = false;
            var target = $(e.target).closest('span, td, th');
            if (target.length === 1) {
                if (!target.is('.disabled')) {
                    switch (target[0].nodeName.toLowerCase()) {
                        case 'th':
                            switch (target[0].className) {
                                case 'switch':
                                    this.showMode(1);
                                    break;
                                case 'prev':
                                case 'next':
                                    var vd = this.viewDate;
                                    var navFnc = DPGlobal.modes[this.viewMode].navFnc;
                                    var step = DPGlobal.modes[this.viewMode].navStep;
                                    if (target[0].className === 'prev') step = step * -1;
                                    vd['set' + navFnc](vd['get' + navFnc]() + step);
                                    this.fillDate();
                                    break;
                            }
                            break;
                        case 'span':
                            if (target.is('.month')) {
                                var month = target.parent().find('span').index(target);
                                this.viewDate.setUTCMonth(month);
                            } else {
                                var year = parseInt(target.text(), 10) || 0;
                                this.viewDate.setUTCFullYear(year);
                            }
                            if (this.viewMode !== 0) {
                                this._date = UTCDate(
                                  this.viewDate.getUTCFullYear(),
                                  this.viewDate.getUTCMonth(),
                                  this.viewDate.getUTCDate(),
                                  this._date.getUTCHours(),
                                  this._date.getUTCMinutes(),
                                  this._date.getUTCSeconds(),
                                  this._date.getUTCMilliseconds()
                                );
                                this.notifyChange();
                            }
                            this.showMode(-1);
                            this.fillDate();
                            break;
                        case 'td':
                            if (target.is('.day')) {
                                var day = parseInt(target.text(), 10) || 1;
                                var month = this.viewDate.getUTCMonth();
                                var year = this.viewDate.getUTCFullYear();
                                if (target.is('.old')) {
                                    if (month === 0) {
                                        month = 11;
                                        year -= 1;
                                    } else {
                                        month -= 1;
                                    }
                                } else if (target.is('.new')) {
                                    if (month == 11) {
                                        month = 0;
                                        year += 1;
                                    } else {
                                        month += 1;
                                    }
                                }
                                this._date = UTCDate(
                                  year, month, day,
                                  this._date.getUTCHours(),
                                  this._date.getUTCMinutes(),
                                  this._date.getUTCSeconds(),
                                  this._date.getUTCMilliseconds()
                                );
                                this.viewDate = UTCDate(
                                  year, month, Math.min(28, day), 0, 0, 0, 0);
                                this.fillDate();
                                this.set();
                                this.notifyChange();
                            }
                            break;
                    }
                }
            }
        },

        actions: {
            incrementHours: function (e) {
                this._date.setUTCHours(this._date.getUTCHours() + 1);
            },

            incrementMinutes: function (e) {
                this._date.setUTCMinutes(this._date.getUTCMinutes() + 1);
            },

            incrementSeconds: function (e) {
                this._date.setUTCSeconds(this._date.getUTCSeconds() + 1);
            },

            decrementHours: function (e) {
                this._date.setUTCHours(this._date.getUTCHours() - 1);
            },

            decrementMinutes: function (e) {
                this._date.setUTCMinutes(this._date.getUTCMinutes() - 1);
            },

            decrementSeconds: function (e) {
                this._date.setUTCSeconds(this._date.getUTCSeconds() - 1);
            },

            togglePeriod: function (e) {
                var hour = this._date.getUTCHours();
                if (hour >= 12) hour -= 12;
                else hour += 12;
                this._date.setUTCHours(hour);
            },

            showPicker: function () {
                this.widget.find('.timepicker > div:not(.timepicker-picker)').hide();
                this.widget.find('.timepicker .timepicker-picker').show();
            },

            showHours: function () {
                this.widget.find('.timepicker .timepicker-picker').hide();
                this.widget.find('.timepicker .timepicker-hours').show();
            },

            showMinutes: function () {
                this.widget.find('.timepicker .timepicker-picker').hide();
                this.widget.find('.timepicker .timepicker-minutes').show();
            },

            showSeconds: function () {
                this.widget.find('.timepicker .timepicker-picker').hide();
                this.widget.find('.timepicker .timepicker-seconds').show();
            },

            selectHour: function (e) {
                var tgt = $(e.target);
                var value = parseInt(tgt.text(), 10);
                if (this.options.pick12HourFormat) {
                    var current = this._date.getUTCHours();
                    if (current >= 12) {
                        if (value != 12) value = (value + 12) % 24;
                    } else {
                        if (value === 12) value = 0;
                        else value = value % 12;
                    }
                }
                this._date.setUTCHours(value);
                this.actions.showPicker.call(this);
            },

            selectMinute: function (e) {
                var tgt = $(e.target);
                var value = parseInt(tgt.text(), 10);
                this._date.setUTCMinutes(value);
                this.actions.showPicker.call(this);
            },

            selectSecond: function (e) {
                var tgt = $(e.target);
                var value = parseInt(tgt.text(), 10);
                this._date.setUTCSeconds(value);
                this.actions.showPicker.call(this);
            }
        },

        doAction: function (e) {
            e.stopPropagation();
            e.preventDefault();
            if (!this._date) this._date = UTCDate(1970, 0, 0, 0, 0, 0, 0);
            var action = $(e.currentTarget).data('action');
            var rv = this.actions[action].apply(this, arguments);
            this.set();
            this.fillTime();
            this.notifyChange();
            return rv;
        },

        stopEvent: function (e) {
            e.stopPropagation();
            e.preventDefault();
        },

        // part of the following code was taken from
        // http://cloud.github.com/downloads/digitalBush/jquery.maskedinput/jquery.maskedinput-1.3.js
        keydown: function (e) {
            var self = this, k = e.which, input = $(e.target);
            if (k == 8 || k == 46) {
                // backspace and delete cause the maskPosition
                // to be recalculated
                setTimeout(function () {
                    self._resetMaskPos(input);
                });
            }
        },

        keypress: function (e) {
            var k = e.which;
            if (k == 8 || k == 46) {
                // For those browsers which will trigger
                // keypress on backspace/delete
                return;
            }
            var input = $(e.target);
            var c = String.fromCharCode(k);
            var val = input.val() || '';
            val += c;
            var mask = this._mask[this._maskPos];
            if (!mask) {
                return false;
            }
            if (mask.end != val.length) {
                return;
            }
            if (!mask.pattern.test(val.slice(mask.start))) {
                val = val.slice(0, val.length - 1);
                while ((mask = this._mask[this._maskPos]) && mask.character) {
                    val += mask.character;
                    // advance mask position past static
                    // part
                    this._maskPos++;
                }
                val += c;
                if (mask.end != val.length) {
                    input.val(val);
                    return false;
                } else {
                    if (!mask.pattern.test(val.slice(mask.start))) {
                        input.val(val.slice(0, mask.start));
                        return false;
                    } else {
                        input.val(val);
                        this._maskPos++;
                        return false;
                    }
                }
            } else {
                this._maskPos++;
            }
        },

        change: function (e) {
            var input = $(e.target);
            var val = input.val();
            if (this._formatPattern.test(val)) {
                this.update();
                this.setValue(this._date.getTime());
                this.notifyChange();
                this.set();
            } else if (val && val.trim()) {
                this.setValue(this._date.getTime());
                if (this._date) this.set();
                else input.val('');
            } else {
                if (this._date) {
                    this.setValue(null);
                    // unset the date when the input is
                    // erased
                    this.notifyChange();
                    this._unset = true;
                }
            }
            this._resetMaskPos(input);
        },

        showMode: function (dir) {
            if (dir) {
                this.viewMode = Math.max(this.minViewMode, Math.min(
                  2, this.viewMode + dir));
            }
            this.widget.find('.datepicker > div').hide().filter(
              '.datepicker-' + DPGlobal.modes[this.viewMode].clsName).show();
        },

        destroy: function () {
            this._detachDatePickerEvents();
            this._detachDatePickerGlobalEvents();
            this.widget.remove();
            this.$element.removeData('datetimepicker');
            this.component.removeData('datetimepicker');
        },

        formatDate: function (d) {
            return this.format.replace(formatReplacer, function (match) {
                var methodName, property, rv, len = match.length;
                if (match === 'ms')
                    len = 1;
                property = dateFormatComponents[match].property;
                if (property === 'Hours12') {
                    rv = d.getUTCHours();
                    if (rv === 0) rv = 12;
                    else if (rv !== 12) rv = rv % 12;
                } else if (property === 'Period12') {
                    if (d.getUTCHours() >= 12) return 'PM';
                    else return 'AM';
                } else {
                    methodName = 'get' + property;
                    rv = d[methodName]();
                }
                if (methodName === 'getUTCMonth') rv = rv + 1;
                if (methodName === 'getUTCYear') rv = rv + 1900 - 2000;
                return padLeft(rv.toString(), len, '0');
            });
        },

        parseDate: function (str) {
            var match, i, property, methodName, value, parsed = {};
            if (!(match = this._formatPattern.exec(str)))
                return null;
            for (i = 1; i < match.length; i++) {
                property = this._propertiesByIndex[i];
                if (!property)
                    continue;
                value = match[i];
                if (/^\d+$/.test(value))
                    value = parseInt(value, 10);
                parsed[property] = value;
            }
            return this._finishParsingDate(parsed);
        },

        _resetMaskPos: function (input) {
            var val = input.val();
            for (var i = 0; i < this._mask.length; i++) {
                if (this._mask[i].end > val.length) {
                    // If the mask has ended then jump to
                    // the next
                    this._maskPos = i;
                    break;
                } else if (this._mask[i].end === val.length) {
                    this._maskPos = i + 1;
                    break;
                }
            }
        },

        _finishParsingDate: function (parsed) {
            var year, month, date, hours, minutes, seconds, milliseconds;
            year = parsed.UTCFullYear;
            if (parsed.UTCYear) year = 2000 + parsed.UTCYear;
            if (!year) year = 1970;
            if (parsed.UTCMonth) month = parsed.UTCMonth - 1;
            else month = 0;
            date = parsed.UTCDate || 1;
            hours = parsed.UTCHours || 0;
            minutes = parsed.UTCMinutes || 0;
            seconds = parsed.UTCSeconds || 0;
            milliseconds = parsed.UTCMilliseconds || 0;
            if (parsed.Hours12) {
                hours = parsed.Hours12;
            }
            if (parsed.Period12) {
                if (/pm/i.test(parsed.Period12)) {
                    if (hours != 12) hours = (hours + 12) % 24;
                } else {
                    hours = hours % 12;
                }
            }
            return UTCDate(year, month, date, hours, minutes, seconds, milliseconds);
        },

        _compileFormat: function () {
            var match, component, components = [], mask = [],
            str = this.format, propertiesByIndex = {}, i = 0, pos = 0;
            while (match = formatComponent.exec(str)) {
                component = match[0];
                if (component in dateFormatComponents) {
                    i++;
                    propertiesByIndex[i] = dateFormatComponents[component].property;
                    components.push('\\s*' + dateFormatComponents[component].getPattern(
                      this) + '\\s*');
                    mask.push({
                        pattern: new RegExp(dateFormatComponents[component].getPattern(
                          this)),
                        property: dateFormatComponents[component].property,
                        start: pos,
                        end: pos += component.length
                    });
                }
                else {
                    components.push(escapeRegExp(component));
                    mask.push({
                        pattern: new RegExp(escapeRegExp(component)),
                        character: component,
                        start: pos,
                        end: ++pos
                    });
                }
                str = str.slice(component.length);
            }
            this._mask = mask;
            this._maskPos = 0;
            this._formatPattern = new RegExp(
              '^\\s*' + components.join('') + '\\s*$');
            this._propertiesByIndex = propertiesByIndex;
        },

        _attachDatePickerEvents: function () {
            var self = this;
            // this handles date picker clicks
            this.widget.on('click', '.datepicker *', $.proxy(this.click, this));
            // this handles time picker clicks
            this.widget.on('click', '[data-action]', $.proxy(this.doAction, this));
            this.widget.on('mousedown', $.proxy(this.stopEvent, this));
            if (this.pickDate && this.pickTime) {
                this.widget.on('click.togglePicker', '.accordion-toggle', function (e) {
                    e.stopPropagation();
                    var $this = $(this);
                    var $parent = $this.closest('ul');
                    var expanded = $parent.find('.in');
                    var closed = $parent.find('.collapse:not(.in)');

                    if (expanded && expanded.length) {
                        var collapseData = expanded.data('collapse');
                        if (collapseData && collapseData.transitioning) return;
                        expanded.collapse('hide');
                        closed.collapse('show');
                        $this.find('span').toggleClass(self.timeIcon + ' ' + self.dateIcon);
                        self.$element.find('.input-group-addon span').toggleClass(self.timeIcon + ' ' + self.dateIcon);
                    }
                });
            }
            if (this.isInput) {
                this.$element.on({
                    'focus': $.proxy(this.show, this),
                    'change': $.proxy(this.change, this),
                    'blur': $.proxy(this.hide, this)
                });
                if (this.options.maskInput) {
                    this.$element.on({
                        'keydown': $.proxy(this.keydown, this),
                        'keypress': $.proxy(this.keypress, this)
                    });
                }
            } else {
                this.$element.on({
                    'change': $.proxy(this.change, this)
                }, 'input');
                if (this.options.maskInput) {
                    this.$element.on({
                        'keydown': $.proxy(this.keydown, this),
                        'keypress': $.proxy(this.keypress, this)
                    }, 'input');
                }
                if (this.component) {
                    this.component.on('click', $.proxy(this.show, this));
                } else {
                    this.$element.on('click', $.proxy(this.show, this));
                }
            }
        },

        _attachDatePickerGlobalEvents: function () {
            $(window).on(
              'resize.datetimepicker' + this.id, $.proxy(this.place, this));
            if (!this.isInput) {
                $(document).on(
                  'mousedown.datetimepicker' + this.id, $.proxy(this.hide, this));
            }
        },

        _detachDatePickerEvents: function () {
            this.widget.off('click', '.datepicker *', this.click);
            this.widget.off('click', '[data-action]');
            this.widget.off('mousedown', this.stopEvent);
            if (this.pickDate && this.pickTime) {
                this.widget.off('click.togglePicker');
            }
            if (this.isInput) {
                this.$element.off({
                    'focus': this.show,
                    'change': this.change
                });
                if (this.options.maskInput) {
                    this.$element.off({
                        'keydown': this.keydown,
                        'keypress': this.keypress
                    });
                }
            } else {
                this.$element.off({
                    'change': this.change
                }, 'input');
                if (this.options.maskInput) {
                    this.$element.off({
                        'keydown': this.keydown,
                        'keypress': this.keypress
                    }, 'input');
                }
                if (this.component) {
                    this.component.off('click', this.show);
                } else {
                    this.$element.off('click', this.show);
                }
            }
        },

        _detachDatePickerGlobalEvents: function () {
            $(window).off('resize.datetimepicker' + this.id);
            if (!this.isInput) {
                $(document).off('mousedown.datetimepicker' + this.id);
            }
        },

        _isInFixed: function () {
            if (this.$element) {
                var parents = this.$element.parents();
                var inFixed = false;
                for (var i = 0; i < parents.length; i++) {
                    if ($(parents[i]).css('position') == 'fixed') {
                        inFixed = true;
                        break;
                    }
                };
                return inFixed;
            } else {
                return false;
            }
        }
    };

    $.fn.datetimepicker = function (option, val) {
        return this.each(function () {
            var $this = $(this),
            data = $this.data('datetimepicker'),
            options = typeof option === 'object' && option;
            if (!data) {
                $this.data('datetimepicker', (data = new DateTimePicker(
                  this, $.extend({}, $.fn.datetimepicker.defaults, options))));
            }
            if (typeof option === 'string') data[option](val);
        });
    };

    $.fn.datetimepicker.defaults = {
        maskInput: false,
        pickDate: true,
        pickTime: true,
        pick12HourFormat: false,
        pickSeconds: true,
        startDate: -Infinity,
        endDate: Infinity,
        collapse: true,
		defaultDate: ""
    };
    $.fn.datetimepicker.Constructor = DateTimePicker;
    var dpgId = 0;
    var dates = $.fn.datetimepicker.dates = {
        en: {
            days: ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday",
              "Friday", "Saturday", "Sunday"],
            daysShort: ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"],
            daysMin: ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"],
            months: ["January", "February", "March", "April", "May", "June",
              "July", "August", "September", "October", "November", "December"],
            monthsShort: ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul",
              "Aug", "Sep", "Oct", "Nov", "Dec"]
        }
    };

    var dateFormatComponents = {
        dd: { property: 'UTCDate', getPattern: function () { return '(0?[1-9]|[1-2][0-9]|3[0-1])\\b'; } },
        MM: { property: 'UTCMonth', getPattern: function () { return '(0?[1-9]|1[0-2])\\b'; } },
        yy: { property: 'UTCYear', getPattern: function () { return '(\\d{2})\\b'; } },
        yyyy: { property: 'UTCFullYear', getPattern: function () { return '(\\d{4})\\b'; } },
        hh: { property: 'UTCHours', getPattern: function () { return '(0?[0-9]|1[0-9]|2[0-3])\\b'; } },
        mm: { property: 'UTCMinutes', getPattern: function () { return '(0?[0-9]|[1-5][0-9])\\b'; } },
        ss: { property: 'UTCSeconds', getPattern: function () { return '(0?[0-9]|[1-5][0-9])\\b'; } },
        ms: { property: 'UTCMilliseconds', getPattern: function () { return '([0-9]{1,3})\\b'; } },
        HH: { property: 'Hours12', getPattern: function () { return '(0?[1-9]|1[0-2])\\b'; } },
        PP: { property: 'Period12', getPattern: function () { return '(AM|PM|am|pm|Am|aM|Pm|pM)\\b'; } }
    };

    var keys = [];
    for (var k in dateFormatComponents) keys.push(k);
    keys[keys.length - 1] += '\\b';
    keys.push('.');

    var formatComponent = new RegExp(keys.join('\\b|'));
    keys.pop();
    var formatReplacer = new RegExp(keys.join('\\b|'), 'g');

    function escapeRegExp(str) {
        // http://stackoverflow.com/questions/3446170/escape-string-for-use-in-javascript-regex
        return str.replace(/[\-\[\]\/\{\}\(\)\*\+\?\.\\\^\$\|]/g, "\\$&");
    }

    function padLeft(s, l, c) {
        if (l < s.length) return s;
        else return Array(l - s.length + 1).join(c || ' ') + s;
    }

    function getTemplate(timeIcon, upIcon, downIcon, pickDate, pickTime, is12Hours, showSeconds, collapse) {
        if (pickDate && pickTime) {
            return (
              '<div class="bootstrap-datetimepicker-widget dropdown-menu" style="z-index:9999 !important;">' +
                '<ul class="list-unstyled">' +
                  '<li' + (collapse ? ' class="collapse in"' : '') + '>' +
                    '<div class="datepicker">' +
                      DPGlobal.template +
                    '</div>' +
                  '</li>' +
                  '<li class="picker-switch accordion-toggle"><a class="btn" style="width:100%"><span class="' + timeIcon + '"></span></a></li>' +
                  '<li' + (collapse ? ' class="collapse"' : '') + '>' +
                    '<div class="timepicker">' +
                      TPGlobal.getTemplate(is12Hours, showSeconds, upIcon, downIcon) +
                    '</div>' +
                  '</li>' +
                '</ul>' +
              '</div>'
            );
        } else if (pickTime) {
            return (
              '<div class="bootstrap-datetimepicker-widget dropdown-menu">' +
                '<div class="timepicker">' +
                  TPGlobal.getTemplate(is12Hours, showSeconds, upIcon, downIcon) +
                '</div>' +
              '</div>'
            );
        } else {
            return (
              '<div class="bootstrap-datetimepicker-widget dropdown-menu">' +
                '<div class="datepicker">' +
                  DPGlobal.template +
                '</div>' +
              '</div>'
            );
        }
    }

    function UTCDate() {
        return new Date(Date.UTC.apply(Date, arguments));
    }

    var DPGlobal = {
        modes: [
          {
              clsName: 'days',
              navFnc: 'UTCMonth',
              navStep: 1
          },
        {
            clsName: 'months',
            navFnc: 'UTCFullYear',
            navStep: 1
        },
        {
            clsName: 'years',
            navFnc: 'UTCFullYear',
            navStep: 10
        }],
        isLeapYear: function (year) {
            return (((year % 4 === 0) && (year % 100 !== 0)) || (year % 400 === 0));
        },
        getDaysInMonth: function (year, month) {
            return [31, (DPGlobal.isLeapYear(year) ? 29 : 28), 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month];
        },
        headTemplate:
          '<thead>' +
            '<tr>' +
              '<th class="prev">&lsaquo;</th>' +
              '<th colspan="5" class="switch"></th>' +
              '<th class="next">&rsaquo;</th>' +
            '</tr>' +
          '</thead>',
        contTemplate: '<tbody><tr><td colspan="7"></td></tr></tbody>'
    };
    DPGlobal.template =
      '<div class="datepicker-days">' +
        '<table class="table-condensed">' +
          DPGlobal.headTemplate +
          '<tbody></tbody>' +
        '</table>' +
      '</div>' +
      '<div class="datepicker-months">' +
        '<table class="table-condensed">' +
          DPGlobal.headTemplate +
          DPGlobal.contTemplate +
        '</table>' +
      '</div>' +
      '<div class="datepicker-years">' +
        '<table class="table-condensed">' +
          DPGlobal.headTemplate +
          DPGlobal.contTemplate +
        '</table>' +
      '</div>';
    var TPGlobal = {
        hourTemplate: '<span data-action="showHours" data-time-component="hours" class="timepicker-hour"></span>',
        minuteTemplate: '<span data-action="showMinutes" data-time-component="minutes" class="timepicker-minute"></span>',
        secondTemplate: '<span data-action="showSeconds" data-time-component="seconds" class="timepicker-second"></span>'
    };
    TPGlobal.getTemplate = function (is12Hours, showSeconds, upIcon, downIcon) {
        return (
        '<div class="timepicker-picker">' +
          '<table class="table-condensed"' +
            (is12Hours ? ' data-hour-format="12"' : '') +
            '>' +
            '<tr>' +
              '<td><a href="#" class="btn" data-action="incrementHours"><span class="' + upIcon + '"></span></a></td>' +
              '<td class="separator"></td>' +
              '<td><a href="#" class="btn" data-action="incrementMinutes"><span class="' + upIcon + '"></span></a></td>' +
              (showSeconds ?
              '<td class="separator"></td>' +
              '<td><a href="#" class="btn" data-action="incrementSeconds"><span class="' + upIcon + '"></span></a></td>' : '') +
              (is12Hours ? '<td class="separator"></td>' : '') +
            '</tr>' +
            '<tr>' +
              '<td>' + TPGlobal.hourTemplate + '</td> ' +
              '<td class="separator">:</td>' +
              '<td>' + TPGlobal.minuteTemplate + '</td> ' +
              (showSeconds ?
              '<td class="separator">:</td>' +
              '<td>' + TPGlobal.secondTemplate + '</td>' : '') +
              (is12Hours ?
              '<td class="separator"></td>' +
              '<td>' +
              '<button type="button" class="btn btn-primary" data-action="togglePeriod"></button>' +
              '</td>' : '') +
            '</tr>' +
            '<tr>' +
              '<td><a href="#" class="btn" data-action="decrementHours"><span class="' + downIcon + '"></span></a></td>' +
              '<td class="separator"></td>' +
              '<td><a href="#" class="btn" data-action="decrementMinutes"><span class="' + downIcon + '"></span></a></td>' +
              (showSeconds ?
              '<td class="separator"></td>' +
              '<td><a href="#" class="btn" data-action="decrementSeconds"><span class="' + downIcon + '"></span></a></td>' : '') +
              (is12Hours ? '<td class="separator"></td>' : '') +
            '</tr>' +
          '</table>' +
        '</div>' +
        '<div class="timepicker-hours" data-action="selectHour">' +
          '<table class="table-condensed">' +
          '</table>' +
        '</div>' +
        '<div class="timepicker-minutes" data-action="selectMinute">' +
          '<table class="table-condensed">' +
          '</table>' +
        '</div>' +
        (showSeconds ?
        '<div class="timepicker-seconds" data-action="selectSecond">' +
          '<table class="table-condensed">' +
          '</table>' +
        '</div>' : '')
        );
    };
})(window.jQuery)
