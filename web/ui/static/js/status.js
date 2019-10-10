
function init() {
  

  $("#get_stats").click(function() {
    // $("#get_stats").attr("disabled", true);
    $("#devHeadStats").html("Loading...")
    $.get("/headstats")
    .fail(function() {
      $("#devHeadStats").html("Error Please try again")
    }).done(function(data) {
      $("#devHeadStats").html(data)
    }).always(function() {
      // $("#get_stats").attr("disabled", false);
    });
  });

}

$(init);
