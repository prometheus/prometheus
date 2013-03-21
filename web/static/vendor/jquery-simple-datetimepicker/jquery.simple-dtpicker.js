/**
 * dtpicker (jquery-simple-datetimepicker)
 * (c) Masanori Ohgita - 2013.
 * https://github.com/mugifly/jquery-simple-datetimepicker
 */

 (function($) {
 	var DAYS_OF_WEEK_EN = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa'];
 	var DAYS_OF_WEEK_JA = ['日', '月', '火', '水', '木', '金', '土'];
 	var MONTHS_EN = [ "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec" ];

 	var PickerObjects = [];
 	var InputObjects = [];
 	var ActivePickerId = -1;

 	var getParentPickerObject = function(obj) {
 		var $obj = $(obj);
 		var $picker;
 		if ($obj.hasClass('datepicker')) {
 			$picker = $obj;
 		} else {
 			var parents = $obj.parents();
 			for (var i = 0; i < parents.length; i++) {
 				if ($(parents[i]).hasClass('datepicker')) {
 					$picker = $(parents[i]);
 				}
 			}
 		}
 		return $picker;
 	};

 	var getPickersInputObject = function($obj) {
 		var $picker = getParentPickerObject($obj);
 		if ($picker.data("inputObjectId") != null) {
 			return $(InputObjects[$picker.data("inputObjectId")]);
 		}
 		return null;
 	}
 	var beforeMonth = function($obj) {
 		var $picker = getParentPickerObject($obj);
 		var date = getPickedDate($picker);
 		var targetMonth_lastDay = new Date(date.getYear() + 1900, date.getMonth(), 0).getDate();
 		if (targetMonth_lastDay < date.getDate()) {
 			date.setDate(targetMonth_lastDay);
 		}
 		draw($picker, {
 			"isAnim": true, 
 			"isOutputToInputObject": true
 		}, date.getYear() + 1900, date.getMonth() - 1, date.getDate(), date.getHours(), date.getMinutes());
 	};

 	var nextMonth = function($obj) {
 		var $picker = getParentPickerObject($obj);
 		var date = getPickedDate($picker);
 		var targetMonth_lastDay = new Date(date.getYear() + 1900, date.getMonth() + 1, 0).getDate();
 		if (targetMonth_lastDay < date.getDate()) {
 			date.setDate(targetMonth_lastDay);
 		}
 		draw($picker, {
 			"isAnim": true, 
 			"isOutputToInputObject": true
 		}, date.getYear() + 1900, date.getMonth() + 1, date.getDate(), date.getHours(), date.getMinutes());
 	};
 	
 	var getDate = function (str) {
 		var re = /^(\d{2,4})[-/](\d{1,2})[-/](\d{1,2}) (\d{1,2}):(\d{1,2})$/;
 		var m = re.exec(str);
		// change year for 4 digits
		if (m[1] < 99) {
			var date = new Date();
			m[1] = parseInt(m[1]) + parseInt(date.getFullYear().toString().substr(0, 2) + "00");
		}
		// return
		return new Date(m[1], m[2] - 1, m[3], m[4], m[5]);
	}

	var outputToInputObject = function($picker) {
		var date = getPickedDate($picker);
		var $inp = getPickersInputObject($picker);
		var dateFormat = $picker.data("dateFormat");
		var locale = $picker.data("locale");
		var str = "";
		if ($inp == null) {
			return;
		}
		
		if (dateFormat == "default"){
			if(locale == "ja"){
				dateFormat = "YYYY/MM/DD hh:mm";
			}else{
				dateFormat = "YYYY-MM-DD hh:mm";
			}
		}
		
		str = dateFormat;
		var y = date.getYear() + 1900;
		var m = date.getMonth() + 1;
		var d = date.getDate();
		var hou = date.getHours();
		var min = date.getMinutes();
		
		str = str.replace(/YYYY/gi, y)
		.replace(/YY/g, y - 2000)/* century */
		.replace(/MM/g, zpadding(m))
		.replace(/M/g, m)
		.replace(/DD/g, zpadding(d))
		.replace(/D/g, d)
		.replace(/hh/g, zpadding(hou))
		.replace(/h/g, hou)
		.replace(/mm/g, zpadding(min))
		.replace(/m/g, min);
		$inp.val(str);
	};

	var getPickedDate = function($obj) {
		var $picker = getParentPickerObject($obj);
		return $picker.data("pickedDate");
	};

	var zpadding = function(num) {
		num = ("0" + num).slice(-2);
		return num
	};

	var draw_date = function($picker, option, date) {
		draw($picker, option, date.getYear() + 1900, date.getMonth(), date.getDate(), date.getHours(), date.getMinutes());
	};

	var draw = function($picker, option, year, month, day, hour, min) {
		var date = new Date();
		
		if (hour != null) {
			date = new Date(year, month, day, hour, min, 0);
		} else if (year != null) {
			date = new Date(year, month, day);
		} else {
			date = new Date();
		}
		//console.log("dtpicker - draw()..." + year + "," + month + "," + day + " " + hour + ":" + min + " -> " + date);
		
		/* Read options */
		var isScroll = option.isAnim; /* It same with isAnim */

		var isAnim = option.isAnim;
		if($picker.data("animation") == false){ // If disabled by user option.
			isAnim = false;
		}
		
		var isOutputToInputObject = option.isOutputToInputObject;

		/* Read locale option */
		var locale = $picker.data("locale");
		var daysOfWeek = DAYS_OF_WEEK_EN;
		if(locale == "ja"){
			daysOfWeek = DAYS_OF_WEEK_JA;
		}
		
		/* Calculate dates */
		var todayDate = new Date(); 
		var firstWday = new Date(date.getYear() + 1900, date.getMonth(), 1).getDay();
		var lastDay = new Date(date.getYear() + 1900, date.getMonth() + 1, 0).getDate();
		var beforeMonthLastDay = new Date(date.getYear() + 1900, date.getMonth(), 0).getDate();
		var dateBeforeMonth = new Date(date.getYear() + 1900, date.getMonth(), 0);
		var dateNextMonth = new Date(date.getYear() + 1900, date.getMonth() + 2, 0);
		
		/* Collect each part */
		var $header = $picker.children('.datepicker_header');
		var $inner = $picker.children('.datepicker_inner_container');
		var $calendar = $picker.children('.datepicker_inner_container').children('.datepicker_calendar');
		var $table = $calendar.children('.datepicker_table');
		var $timelist = $picker.children('.datepicker_inner_container').children('.datepicker_timelist');
		
		/* Grasp a point that will be changed */
		var changePoint = "";
		var oldDate = getPickedDate($picker);
		if(oldDate != null){
			if(oldDate.getMonth() != date.getMonth() || oldDate.getDate() != date.getDate()){
				changePoint = "calendar";
			} else if (oldDate.getHours() != date.getHours() || oldDate.getMinutes() != date.getMinutes()){
				if(date.getMinutes() == 0 || date.getMinutes() == 30){
					changePoint = "timelist";
				}
			}
		}
		
		/* Save newly date to Picker data */
		$($picker).data("pickedDate", date);
		
		/* Fade-out animation */
		if (isAnim == true) {
			if(changePoint == "calendar"){
				$calendar.stop().queue([]);
				$calendar.fadeTo("fast", 0.8);
			}else if(changePoint == "timelist"){
				$timelist.stop().queue([]);
				$timelist.fadeTo("fast", 0.8);
			}
		}
		/* Remind timelist scroll state */
		var drawBefore_timeList_scrollTop = $timelist.scrollTop();
		
		/* New timelist  */
		var timelist_activeTimeCell_offsetTop = -1;
		
		/* Header ----- */
		$header.children().remove();
		var $link_before_month = $('<a>');
		$link_before_month.text('<');
		$link_before_month.click(function() {
			beforeMonth($picker);
		});

		var $now_month = $('<span>');
		if(locale == "en"){
			$now_month.text((date.getYear() + 1900) + " - " + MONTHS_EN[date.getMonth()]);
		}else if(locale == "ja"){
			$now_month.text((date.getYear() + 1900) + " / " + zpadding(date.getMonth() + 1));
		}

		var $link_next_month = $('<a>');
		$link_next_month.text('>');
		$link_next_month.click(function() {
			nextMonth($picker);
		});

		$header.append($link_before_month);
		$header.append($now_month);
		$header.append($link_next_month);

		/* Calendar > Table ----- */
		$table.children().remove();
		var $tr = $('<tr>');
		$table.append($tr);

		/* Output wday cells */
		for (var i = 0; i < 7; i++) {
			var $td = $('<th>');
			$td.text(daysOfWeek[i]);
			$tr.append($td);
		}

		/* Output day cells */
		var cellNum = Math.ceil((firstWday + lastDay) / 7) * 7;
		for (var i = 0; i < cellNum; i++) {
			var realDay = i + 1 - firstWday;
			if (i % 7 == 0) {
				$tr = $('<tr>');
				$table.append($tr);
			}

			var $td = $('<td>');
			$td.data("day", realDay);
			
			$tr.append($td);
			
			if (firstWday > i) {/* Before months day */
				$td.text(beforeMonthLastDay + realDay);
				$td.addClass('day_another_month');
				$td.data("dateStr", dateBeforeMonth.getYear() + 1900 + "/" + (dateBeforeMonth.getMonth() + 1) + "/" + (beforeMonthLastDay + realDay));
			} else if (i < firstWday + lastDay) {/* Now months day */
				$td.text(realDay);
				$td.data("dateStr", (date.getYear() + 1900) + "/" + (date.getMonth() + 1) + "/" + realDay);
			} else {/* Next months day */
				$td.text(realDay - lastDay);
				$td.addClass('day_another_month');
				$td.data("dateStr", dateNextMonth.getYear() + 1900 + "/" + (dateNextMonth.getMonth() + 1) + "/" + (realDay - lastDay));
			}

			if (i % 7 == 0) {/* Sunday */
				$td.addClass('wday_sun');
			} else if (i % 7 == 6) {/* Saturday */
				$td.addClass('wday_sat');
			}

			if (realDay == date.getDate()) {/* selected day */
				$td.addClass('active');
			}
			
			if (date.getMonth() == todayDate.getMonth() && realDay == todayDate.getDate()) {/* today */
				$td.addClass('today');
			}

			/* Set event-handler to day cell */

			$td.click(function() {
				if ($(this).hasClass('hover')) {
					$(this).removeClass('hover');
				}
				$(this).addClass('active');

				var $picker = getParentPickerObject($(this));
				var targetDate = new Date($(this).data("dateStr"));
				var selectedDate = getPickedDate($picker);
				draw($picker, {
					"isAnim": false, 
					"isOutputToInputObject": true
				}, targetDate.getYear() + 1900, targetDate.getMonth(), targetDate.getDate(), selectedDate.getHours(), selectedDate.getMinutes());
			});

			$td.hover(function() {
				if (! $(this).hasClass('active')) {
					$(this).addClass('hover');
				}
			}, function() {
				if ($(this).hasClass('hover')) {
					$(this).removeClass('hover');
				}
			});
		}

		/* Timelist ----- */
		$timelist.children().remove();
		
		/* Set height to Timelist (Calendar innerHeight - Calendar padding) */
		$timelist.css("height", $calendar.innerHeight() - 10 + 'px');

		/* Output time cells */
		for (var hour = 0; hour < 24; hour++) {
			for (var min = 0; min <= 30; min += 30) {
				var $o = $('<div>');
				$o.addClass('timelist_item');
				$o.text(zpadding(hour) + ":" + zpadding(min));

				$o.data("hour", hour);
				$o.data("min", min);

				$timelist.append($o);

				if (hour == date.getHours() && min == date.getMinutes()) {/* selected time */
					$o.addClass('active');
					timelist_activeTimeCell_offsetTop = $o.offset().top;
				}

				/* Set event handler to time cell */
				
				$o.click(function() {
					if ($(this).hasClass('hover')) {
						$(this).removeClass('hover');
					}
					$(this).addClass('active');

					var $picker = getParentPickerObject($(this));
					var date = getPickedDate($picker);
					var hour = $(this).data("hour");
					var min = $(this).data("min");
					draw($picker, {
						"isAnim": false, 
						"isOutputToInputObject": true
					}, date.getYear() + 1900, date.getMonth(), date.getDate(), hour, min);
				});
				
				$o.hover(function() {
					if (! $(this).hasClass('active')) {
						$(this).addClass('hover');
					}
				}, function() {
					if ($(this).hasClass('hover')) {
						$(this).removeClass('hover');
					}
				});
			}
		}
		
		/* Scroll the timelist */
		if(isScroll == true){
			/* Scroll to new active time-cell position */
			$timelist.scrollTop(timelist_activeTimeCell_offsetTop - $timelist.offset().top);
		}else{
			/* Scroll to position that before redraw. */
			$timelist.scrollTop(drawBefore_timeList_scrollTop);
		}

		/* Fade-in animation */
		if (isAnim == true) {
			if(changePoint == "calendar"){
				$calendar.fadeTo("fast", 1.0);
			}else if(changePoint == "timelist"){
				$timelist.fadeTo("fast", 1.0);
			}
		}

		/* Output to InputForm */
		if (isOutputToInputObject == true) {
			outputToInputObject($picker);
		}
	};

	var init = function($obj, opt) {
		/* Container */
		var $picker = $('<div>');
		$picker.addClass('datepicker')
		$obj.append($picker);
		
		/* Set options data to container object  */
		if (opt.inputObjectId != null) {
			$picker.data("inputObjectId", opt.inputObjectId);
		}
		$picker.data("pickerId", PickerObjects.length);
		$picker.data("dateFormat", opt.dateFormat);
		$picker.data("locale", opt.locale);
		$picker.data("animation", opt.animation);

		/* Header */
		var $header = $('<div>');
		$header.addClass('datepicker_header');
		$picker.append($header);
		/* InnerContainer*/
		var $inner = $('<div>');
		$inner.addClass('datepicker_inner_container');
		$picker.append($inner);
		/* Calendar */
		var $calendar = $('<div>');
		$calendar.addClass('datepicker_calendar');
		var $table = $('<table>');
		$table.addClass('datepicker_table');
		$calendar.append($table);
		$inner.append($calendar);
		/* Timelist */
		var $timelist = $('<div>');
		$timelist.addClass('datepicker_timelist');
		$inner.append($timelist);

		/* Set event handler to picker */
		$picker.hover(
			function(){
				ActivePickerId = $(this).data("pickerId");
			},
			function(){
				ActivePickerId = -1;
			}
			);

		PickerObjects.push($picker);
		
		draw_date($picker, {
			"isAnim": true, 
			"isOutputToInputObject": true
		}, opt.current);
	};

	/**
	 * Initialize dtpicker
	 */
	 $.fn.dtpicker = function(config) {
	 	var date = new Date();
	 	var defaults = {
	 		"inputObjectId": 	undefined,
	 		"current": 		date.getFullYear() + '-' + (date.getMonth()+1) + '-' + date.getDate() + ' ' + date.getHours() + ':' + date.getMinutes(),
	 		"dateFormat": 	"default",
	 		"locale": 			"en",
	 		"animation":           true
	 	};
	 	
	 	var options = $.extend(defaults, config);
	 	options.current = getDate(options.current);
	 	return this.each(function(i) {
	 		init($(this), options);
	 	});
	 };

	/**
	 * Initialize dtpicker, append to Text input field
	 * */
	 $.fn.appendDtpicker = function(config) {
	 	var date = new Date();
	 	var defaults = {
	 		"inline": false,
	 		"current": date.getFullYear() + '-' + (date.getMonth()+1) + '-' + date.getDate() + ' ' + date.getHours() + ':' + date.getMinutes(),
	 		"dateFormat": "default",
	 		"locale": 			"en",
	 		"animation": true
	 	}
	 	var options = $.extend(defaults, config);
	 	return this.each(function(i) {

	 		/* Add input-field with inputsObjects array */
	 		var input = this;
	 		var inputObjectId = InputObjects.length;
	 		InputObjects.push(input);
	 		
	 		options.inputObjectId = inputObjectId;
	 		
	 		/* Current date */
	 		var date, strDate, strTime;
	 		if($(input).val() != null && $(input).val() != ""){
	 			options.current = $(input).val();
	 		}
	 		
	 		/* Make parent-div for picker */
	 		var $d = $('<div>');
	 		if(options.inline == false){
	 			/* float mode */
	 			$d.css("position","absolute");
	 		}
	 		$d.insertAfter(input);
	 		
	 		/* Initialize picker */
	 		
	 		var pickerId = PickerObjects.length;
	 		
			var $picker_parent = $($d).dtpicker(options); // call dtpicker() method
			
			var $picker = $picker_parent.children('.datepicker');

			/* Link input-field with picker*/
			$(input).data('pickerId', pickerId);
			
			/* Set event handler to input-field */
			
			$(input).keyup(function() {
				var $input = $(this);
				var $picker = $(PickerObjects[$input.data('pickerId')]);
				if ($input.val() != null && (
					$input.data('beforeVal') == null ||
					( $input.data('beforeVal') != null && $input.data('beforeVal') != $input.val())	)
					) { /* beforeValue == null || beforeValue != nowValue  */
					var date = getDate($input.val());
				if (isNaN(date.getDate()) == false) {/* Valid format... */
					draw_date($picker, {
						"isAnim":true, 
						"isOutputToInputObject":false
					}, date);
				}
			}
			$input.data('beforeVal',$input.val())
		});
			
			$(input).change(function(){
				$(this).trigger('keyup');
			});
			
			if(options.inline == true){
				/* inline mode */
				$picker.data('isInline',true);
			}else{
				/* float mode */
				$picker.data('isInline',false);
				$picker_parent.css({
					"zIndex": 100
				});
				$picker.css("width","auto");
				
				/* Hide this picker */
				$picker.hide();
				
				/* Set onClick event handler for input-field */
				$(input).click(function(){
					var $input = $(this);
					var $picker = $(PickerObjects[$input.data('pickerId')]);
					ActivePickerId = $input.data('pickerId');
					$picker.show();
					$picker.parent().css("top", $input.offset().top + $input.outerHeight() + 2 + "px");
					$picker.parent().css("left", $input.offset().left + "px");
				});
			}
		});
};

/* Set event handler to Body element, for hide a floated-picker */
$(function(){
	$('body').click(function(){
		for(var i=0;i<PickerObjects.length;i++){
			var $picker = $(PickerObjects[i]);
			if(ActivePickerId != i){	/* if not-active picker */
				if($picker.data("inputObjectId") != null && $picker.data("isInline") == false){
					/* if append input-field && float picker */
					$picker.hide();
				}
			}
		}
	});
});

})(jQuery);
