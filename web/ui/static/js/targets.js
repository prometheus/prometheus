function init() {
  $('.job_header').click(function() {
    var selector = $(this).attr('data-target');
    localStorage.setItem(selector, !$(selector).hasClass('in'));
  });

  $('.job_header').each(function(i, obj) {
    var selector = $(obj).attr('data-target');
    if (localStorage.getItem(selector) === "true" || localStorage.getItem(selector) === null) {
      $(selector).addClass('in');
    }
  })
}

$(init);
