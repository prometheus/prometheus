function init() {
  $(".alert_header").click(function() {
    var expanderIcon = $(this).find("i.icon-chevron-down");
    if (expanderIcon.length !== 0) {
      expanderIcon.removeClass("icon-chevron-down").addClass("icon-chevron-up");
    } else {
      var collapserIcon = $(this).find("i.icon-chevron-up");
      collapserIcon.removeClass("icon-chevron-up").addClass("icon-chevron-down");
    }
    $(this).next().toggle();
  });

  $("div.show-annotations").click(function() {
    const targetEl = $('div.show-annotations');
    const icon = $(targetEl).children('i');

    if (icon.hasClass('glyphicon-unchecked')) {
        $(".alert_annotations").show();
        $(".alert_annotations_header").show();
        $(targetEl).children('i').removeClass('glyphicon-unchecked').addClass('glyphicon-check');
        targetEl.addClass('is-checked');
    } else if (icon.hasClass('glyphicon-check')) {
        $(".alert_annotations").hide();
        $(".alert_annotations_header").hide();
        $(targetEl).children('i').removeClass('glyphicon-check').addClass('glyphicon-unchecked');
        targetEl.removeClass('is-checked');
    }
  });
}

$(init);
